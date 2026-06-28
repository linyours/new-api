package service

import (
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

var invalidThinkingSignaturePatterns = []*regexp.Regexp{
	regexp.MustCompile("(?i)invalid `signature` in `thinking` block"),
	regexp.MustCompile("(?i)(?:^|\\s|\\()\\S*\\.signature:\\s*field\\s+required(?:\\s|\\)|$)"),
	regexp.MustCompile("(?i)(?:^|\\s|\\()\\S*\\.thinking:\\s*field\\s+required(?:\\s|\\)|$)"),
}

var emptyTextContentBlocksPatterns = []*regexp.Regexp{
	regexp.MustCompile("(?i)messages:\\s*text content blocks must be non-empty"),
}

const (
	claudeContentTypeText             = "text"
	claudeContentTypeThinking         = "thinking"
	claudeContentTypeRedactedThinking = "redacted_thinking"
	claudeContentPlaceholderText      = "."
)

func IsInvalidThinkingSignatureError(statusCode int, errMsg string) bool {
	for _, pattern := range invalidThinkingSignaturePatterns {
		if pattern.MatchString(errMsg) {
			return true
		}
	}
	return false
}

func IsEmptyTextContentBlocksError(statusCode int, errMsg string) bool {
	for _, pattern := range emptyTextContentBlocksPatterns {
		if pattern.MatchString(errMsg) {
			return true
		}
	}
	return false
}

// FixClaudeRequestOnFirstRetry applies a targeted self-healing strategy.
// FixClaudeRequestOnFirstRetry 实现“定向自愈”策略。
// It is triggered by recognized upstream error messages and may apply one or
// both fixes:
// - fill empty text blocks with a non-whitespace placeholder;
// - strip thinking blocks when signature is invalid.
// 按错误信息命中规则执行一个或多个修复：
// - 将空 text 块填充为非空白占位符；
// - 在 thinking signature 无效时剥离 thinking block。
func FixClaudeRequestOnFirstRetry(request *dto.ClaudeRequest, statusCode int, errMsg string) int {
	if request == nil {
		return 0
	}
	changedCount := 0
	if IsEmptyTextContentBlocksError(statusCode, errMsg) {
		changedCount += fillEmptyTextBlocks(request)
	}
	if IsInvalidThinkingSignatureError(statusCode, errMsg) {
		changedCount += stripThinkingBlocks(request)
	}
	return changedCount
}

func fillEmptyTextBlocks(request *dto.ClaudeRequest) int {
	if request == nil || len(request.Messages) == 0 {
		return 0
	}
	changedCount := 0
	for idx := range request.Messages {
		changedCount += fillMessageEmptyTextBlocks(&request.Messages[idx])
	}
	return changedCount
}

func fillMessageEmptyTextBlocks(message *dto.ClaudeMessage) int {
	if message == nil || message.Content == nil {
		return 0
	}
	switch content := message.Content.(type) {
	case []any:
		changed := 0
		for idx := range content {
			if fillBlockEmptyTextAny(content[idx]) {
				changed++
			}
		}
		message.Content = content
		return changed
	case []dto.ClaudeMediaMessage:
		changed := 0
		for idx := range content {
			if content[idx].Type != claudeContentTypeText || strings.TrimSpace(content[idx].GetText()) != "" {
				continue
			}
			content[idx].SetText(claudeContentPlaceholderText)
			changed++
		}
		message.Content = content
		return changed
	default:
		generic, err := common.Any2Type[[]any](message.Content)
		if err != nil {
			return 0
		}
		changed := 0
		for idx := range generic {
			if fillBlockEmptyTextAny(generic[idx]) {
				changed++
			}
		}
		message.Content = generic
		return changed
	}
}

func fillBlockEmptyTextAny(block any) bool {
	blockMap, ok := block.(map[string]any)
	if !ok {
		return false
	}
	typeValue, _ := blockMap["type"].(string)
	if strings.TrimSpace(typeValue) != claudeContentTypeText {
		return false
	}
	textValue, _ := blockMap["text"].(string)
	if strings.TrimSpace(textValue) != "" {
		return false
	}
	blockMap["text"] = claudeContentPlaceholderText
	return true
}

func stripThinkingBlocks(request *dto.ClaudeRequest) int {
	if request == nil || len(request.Messages) == 0 {
		return 0
	}
	strippedCount := 0
	for idx := range request.Messages {
		strippedCount += stripMessageThinkingBlocks(&request.Messages[idx])
	}
	return strippedCount
}

func stripMessageThinkingBlocks(message *dto.ClaudeMessage) int {
	if message == nil || message.Content == nil {
		return 0
	}
	switch content := message.Content.(type) {
	case []any:
		before := len(content)
		filtered := make([]any, 0, before)
		for _, block := range content {
			if isThinkingBlock(block) {
				continue
			}
			filtered = append(filtered, block)
		}
		removed := before - len(filtered)
		// Keep content non-empty to avoid triggering "empty content" style 400s
		// after stripping all reasoning blocks.
		// 保持 content 非空，避免剥离推理块后再次触发“content 为空”类 400。
		ensureMessageContentPlaceholderAny(&filtered, before)
		message.Content = filtered
		return removed
	case []dto.ClaudeMediaMessage:
		before := len(content)
		filtered := make([]dto.ClaudeMediaMessage, 0, before)
		for _, block := range content {
			if isThinkingType(block.Type) {
				continue
			}
			filtered = append(filtered, block)
		}
		removed := before - len(filtered)
		// Keep content non-empty to avoid triggering "empty content" style 400s
		// after stripping all reasoning blocks.
		// 保持 content 非空，避免剥离推理块后再次触发“content 为空”类 400。
		ensureMessageContentPlaceholderTyped(&filtered, before)
		message.Content = filtered
		return removed
	default:
		generic, err := common.Any2Type[[]any](message.Content)
		if err != nil {
			return 0
		}
		before := len(generic)
		filtered := make([]any, 0, before)
		for _, block := range generic {
			if isThinkingBlock(block) {
				continue
			}
			filtered = append(filtered, block)
		}
		removed := before - len(filtered)
		// Keep content non-empty to avoid triggering "empty content" style 400s
		// after stripping all reasoning blocks.
		// 保持 content 非空，避免剥离推理块后再次触发“content 为空”类 400。
		ensureMessageContentPlaceholderAny(&filtered, before)
		message.Content = filtered
		return removed
	}
}

func isThinkingBlock(block any) bool {
	blockMap, ok := block.(map[string]any)
	if !ok {
		return false
	}
	typeValue, _ := blockMap["type"].(string)
	return isThinkingType(typeValue)
}

func isThinkingType(blockType string) bool {
	switch strings.TrimSpace(blockType) {
	case claudeContentTypeThinking, claudeContentTypeRedactedThinking:
		return true
	default:
		return false
	}
}

func ensureMessageContentPlaceholderAny(content *[]any, before int) {
	if before <= 0 || len(*content) > 0 {
		return
	}
	*content = append(*content, map[string]any{
		"type": claudeContentTypeText,
		"text": claudeContentPlaceholderText,
	})
}

func ensureMessageContentPlaceholderTyped(content *[]dto.ClaudeMediaMessage, before int) {
	if before <= 0 || len(*content) > 0 {
		return
	}
	placeholder := dto.ClaudeMediaMessage{Type: claudeContentTypeText}
	placeholder.SetText(claudeContentPlaceholderText)
	*content = append(*content, placeholder)
}
