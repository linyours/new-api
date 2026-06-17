package helper

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
)

const (
	InitialScannerBufferSize    = 64 << 10  // 64KB (64*1024)
	DefaultMaxScannerBufferSize = 128 << 20 // 64MB (64*1024*1024) default SSE buffer size
	DefaultPingInterval         = 10 * time.Second
	// streamWriteTimeout bounds a single blocked write to a slow client so the
	// unconditional wg.Wait() in cleanup can always finish. Without it, a slow
	// but connected client (full TCP buffer, no server WriteTimeout) could hang
	// the handler forever.
	streamWriteTimeout = 30 * time.Second
)

func getScannerBufferSize() int {
	if constant.StreamScannerMaxBufferMB > 0 {
		return constant.StreamScannerMaxBufferMB << 20
	}
	return DefaultMaxScannerBufferSize
}

func NewStreamScanner(reader io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, InitialScannerBufferSize), getScannerBufferSize())
	return scanner
}

func copyCodexSSEHeaders(c *gin.Context, resp *http.Response) {
	if c == nil || c.Writer == nil || resp == nil {
		return
	}
	// codex
	for _, name := range []string{"X-Reasoning-Included", "X-Codex-Turn-State"} {
		values := resp.Header.Values(name)
		if !service.ShouldCopyUpstreamHeader(c, name, values) {
			continue
		}
		for _, value := range values {
			if value != "" {
				c.Writer.Header().Add(name, value)
			}
		}
	}
}

// ExtendWriteDeadline pushes the connection write deadline forward before each
// stream write. Best-effort: writers that don't support deadlines (e.g.
// httptest recorders) are silently ignored.
func ExtendWriteDeadline(c *gin.Context) {
	if c == nil || c.Writer == nil {
		return
	}
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(streamWriteTimeout))
}

func sendTTFTImmediateWebhookAlert(c *gin.Context, info *relaycommon.RelayInfo, detail string) {
	if c == nil || info == nil {
		return
	}

	channelID := info.ChannelId
	tokenID := info.TokenId
	modelName := info.OriginModelName
	if modelName == "" {
		modelName = info.UpstreamModelName
	}
	requestID := info.RequestId
	if requestID == "" {
		requestID = c.GetString(common.RequestIdKey)
	}
	requestPath := "-"
	if c.Request != nil && c.Request.URL != nil && c.Request.URL.Path != "" {
		requestPath = c.Request.URL.Path
	}
	group := info.UsingGroup
	if group == "" {
		group = c.GetString("group")
	}
	tokenName := c.GetString("token_name")
	alertTime := time.Now().Format("2006-01-02 15:04:05")

	gopool.Go(func() {
		cfg, err := service.GetTTFTMonitorConfig()
		if err != nil || !cfg.Enabled || cfg.NotifyType != "webhook" || strings.TrimSpace(cfg.WebhookURL) == "" {
			return
		}

		subject := fmt.Sprintf("【异常告警】通道 #%d TTFT 超阈值", channelID)
		content := fmt.Sprintf(
			"错误级别：严重\n"+
				"告警类型：TTFT 即时告警\n"+
				"模型信息：%s\n"+
				"分组信息：%s\n"+
				"渠道信息：channelId=%d, tokenId=%d, tokenName=%s\n"+
				"请求路径：%s\n"+
				"requestId：%s\n"+
				"告警详情：%s\n"+
				"告警时间：%s",
			modelName,
			group,
			channelID,
			tokenID,
			tokenName,
			requestPath,
			requestID,
			detail,
			alertTime,
		)
		notify := dto.NewNotify(
			fmt.Sprintf("%s_%d", dto.NotifyTypeTTFTAlert, channelID),
			subject,
			content,
			nil,
		)
		if err = service.SendWebhookNotify(cfg.WebhookURL, cfg.WebhookSecret, notify); err != nil {
			common.SysLog(fmt.Sprintf("ttft immediate webhook notify failed: %v", err))
		}
	})
}

func StreamScannerHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, dataHandler func(data string, sr *StreamResult)) {

	if resp == nil || dataHandler == nil {
		return
	}

	// 无条件新建 StreamStatus
	info.StreamStatus = relaycommon.NewStreamStatus()

	ctx, cancel := context.WithCancel(context.Background())

	streamingTimeout := time.Duration(constant.StreamingTimeout) * time.Second
	// ===================== BEGIN NEW: TTFT timeout config =====================
	ttftTimeout := time.Duration(constant.TTFTTimeoutSeconds) * time.Second
	// ====================== END NEW: TTFT timeout config ======================

	var (
		stopChan    = make(chan bool, 3) // 增加缓冲区避免阻塞
		scanner     = NewStreamScanner(resp.Body)
		ticker      = time.NewTicker(streamingTimeout)
		pingTicker  *time.Ticker
		writeMutex  sync.Mutex     // Mutex to protect concurrent writes
		wg          sync.WaitGroup // 用于等待所有 goroutine 退出
		cleanupOnce sync.Once
		stopOnce    sync.Once
		ttftTimer     *time.Timer
		ttftTimeoutCh <-chan time.Time
		stopTTFTOnce  sync.Once
	)

	stop := func() {
		stopOnce.Do(func() {
			close(stopChan)
		})
	}

	// Stop + drain to avoid timer channel leaks / stale trigger after stop.
	stopTTFTTimer := func() {
		stopTTFTOnce.Do(func() {
			if ttftTimer == nil {
				return
			}
			if !ttftTimer.Stop() {
				select {
				case <-ttftTimer.C:
				default:
				}
			}
		})
	}

	// A nil channel never fires in select, so disabled mode has near-zero
	// runtime overhead.
	if constant.TTFTTimeoutSeconds > 0 && ttftTimeout > 0 {
		startTime := info.StartTime
		if startTime.IsZero() {
			startTime = time.Now()
		}
		elapsed := time.Since(startTime)
		remaining := ttftTimeout - elapsed
		if remaining <= 0 {
			sendTTFTImmediateWebhookAlert(
				c,
				info,
				fmt.Sprintf(
					"ttft timeout exceeded before scanner start (elapsed=%s, limit=%s)",
					elapsed.Truncate(time.Millisecond),
					ttftTimeout,
				),
			)
		} else {
			ttftTimer = time.NewTimer(remaining)
			ttftTimeoutCh = ttftTimer.C
		}
	}

	generalSettings := operation_setting.GetGeneralSetting()
	pingEnabled := generalSettings.PingIntervalEnabled && !info.DisablePing
	pingInterval := time.Duration(generalSettings.PingIntervalSeconds) * time.Second
	if pingInterval <= 0 {
		pingInterval = DefaultPingInterval
	}

	if pingEnabled {
		pingTicker = time.NewTicker(pingInterval)
	}

	logger.LogDebug(c, "relay timeout seconds: %d", common.RelayTimeout)
	logger.LogDebug(c, "relay max idle conns: %d", common.RelayMaxIdleConns)
	logger.LogDebug(c, "relay max idle conns per host: %d", common.RelayMaxIdleConnsPerHost)
	logger.LogDebug(c, "streaming timeout seconds: %d", int64(streamingTimeout.Seconds()))
	logger.LogDebug(c, "ttft timeout seconds: %d", int64(ttftTimeout.Seconds()))
	logger.LogDebug(c, "ping interval seconds: %d", int64(pingInterval.Seconds()))

	cleanup := func() {
		cleanupOnce.Do(func() {
			stopTTFTTimer()
			cancel()
			stop()
			if resp.Body != nil {
				_ = resp.Body.Close()
			}

			ticker.Stop()
			if pingTicker != nil {
				pingTicker.Stop()
			}

			wg.Wait()
		})
	}
	// Ensure gin.Context is not returned to Gin's pool while any stream goroutine can still use it.
	defer cleanup()

	scanner.Split(bufio.ScanLines)
	copyCodexSSEHeaders(c, resp)
	SetEventStreamHeaders(c)

	ctx = context.WithValue(ctx, "stop_chan", stopChan)

	// Handle ping data sending with improved error handling
	if pingEnabled && pingTicker != nil {
		wg.Add(1)
		gopool.Go(func() {
			defer func() {
				if r := recover(); r != nil {
					logger.LogError(c, fmt.Sprintf("ping goroutine panic: %v", r))
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("ping panic: %v", r))
					stop()
				}
				logger.LogDebug(c, "ping goroutine exited")
				wg.Done()
			}()

			// 添加超时保护，防止 goroutine 无限运行
			maxPingDuration := 30 * time.Minute // 最大 ping 持续时间
			pingTimeout := time.NewTimer(maxPingDuration)
			defer pingTimeout.Stop()

			for {
				select {
				case <-pingTicker.C:
					var err error
					func() {
						writeMutex.Lock()
						defer writeMutex.Unlock()
						ExtendWriteDeadline(c)
						err = PingData(c)
					}()
					if err != nil {
						logger.LogError(c, "ping data error: "+err.Error())
						info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPingFail, err)
						return
					}
					logger.LogDebug(c, "ping data sent")
				case <-ctx.Done():
					return
				case <-stopChan:
					return
				case <-c.Request.Context().Done():
					// 监听客户端断开连接
					return
				case <-pingTimeout.C:
					logger.LogError(c, "ping goroutine max duration reached")
					return
				}
			}
		})
	}

	dataChan := make(chan string, 10)

	wg.Add(1)
	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogError(c, fmt.Sprintf("data handler goroutine panic: %v", r))
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("handler panic: %v", r))
			}
			stop()
			wg.Done()
		}()
		sr := newStreamResult(info.StreamStatus)
		for data := range dataChan {
			sr.reset()
			func() {
				writeMutex.Lock()
				defer writeMutex.Unlock()
				ExtendWriteDeadline(c)
				dataHandler(data, sr)
			}()
			if sr.IsStopped() {
				return
			}
		}
	})

	// Scanner goroutine with improved error handling
	wg.Add(1)
	common.RelayCtxGo(ctx, func() {
		defer func() {
			close(dataChan)
			if r := recover(); r != nil {
				logger.LogError(c, fmt.Sprintf("scanner goroutine panic: %v", r))
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("scanner panic: %v", r))
			}
			stop()
			logger.LogDebug(c, "scanner goroutine exited")
			wg.Done()
		}()

		for scanner.Scan() {
			// 检查是否需要停止
			select {
			case <-stopChan:
				return
			case <-ctx.Done():
				return
			default:
			}

			ticker.Reset(streamingTimeout)
			data := scanner.Text()
			logger.LogDebug(c, "stream scanner data: %s", data)

			if len(data) < 6 {
				continue
			}
			if data[:5] != "data:" && data[:6] != "[DONE]" {
				continue
			}
			data = data[5:]
			data = strings.TrimSpace(data)
			if data == "" {
				continue
			}
			if !strings.HasPrefix(data, "[DONE]") {
				// ===================== BEGIN NEW: stop TTFT on first valid chunk =====================
				// Use received count as first-chunk marker to avoid additional parsing
				// cost on the hot path.
				if info.ReceivedResponseCount == 0 {
					stopTTFTTimer()
				}
				// ====================== END NEW: stop TTFT on first valid chunk ======================
				info.SetFirstResponseTime()
				info.ReceivedResponseCount++

				select {
				case dataChan <- data:
				case <-ctx.Done():
					return
				case <-stopChan:
					return
				}
			} else {
				// ===================== BEGIN NEW: stop TTFT on DONE =====================
				stopTTFTTimer()
				// ====================== END NEW: stop TTFT on DONE ======================
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
				logger.LogDebug(c, "received [DONE], stopping scanner")
				return
			}
		}

		// ===================== BEGIN NEW: stop TTFT on scanner exit =====================
		stopTTFTTimer()
		// ====================== END NEW: stop TTFT on scanner exit ======================

		if err := scanner.Err(); err != nil {
			if err != io.EOF {
				logger.LogError(c, "scanner error: "+err.Error())
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
			}
		}
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonEOF, nil)
	})

	// 主循环等待完成或超时
	waitForStreamEnd := false
	select {
	case <-ticker.C:
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, nil)
	// ===================== BEGIN NEW: TTFT timeout branch =====================
	case <-ttftTimeoutCh:
		sendTTFTImmediateWebhookAlert(
			c,
			info,
			fmt.Sprintf("ttft timeout: no first chunk within %d seconds", constant.TTFTTimeoutSeconds),
		)
		ttftTimeoutCh = nil
		waitForStreamEnd = true
	// ====================== END NEW: TTFT timeout branch ======================
	case <-stopChan:
		// EndReason already set by the goroutine that triggered stopChan
	case <-c.Request.Context().Done():
		// 客户端断开：立即 cleanup 关闭上游 resp.Body，解除 scanner 阻塞并让上游停止生成，
		// 避免为已放弃的请求继续消费上游 token。
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, c.Request.Context().Err())
	}
	if waitForStreamEnd {
		select {
		case <-stopChan:
			// EndReason already set by the goroutine that triggered stopChan
		case <-ticker.C:
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, nil)
		case <-c.Request.Context().Done():
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, c.Request.Context().Err())
		}
	}

	cleanup()
	if info.StreamStatus.IsNormalEnd() && !info.StreamStatus.HasErrors() {
		logger.LogInfo(c, fmt.Sprintf("stream ended: %s", info.StreamStatus.Summary()))
	} else {
		logger.LogError(c, fmt.Sprintf("stream ended: %s, received=%d", info.StreamStatus.Summary(), info.ReceivedResponseCount))
	}
}
