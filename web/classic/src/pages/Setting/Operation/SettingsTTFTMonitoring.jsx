/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useRef, useState } from 'react';
import { Banner, Button, Col, Form, Row, Spin, TagInput } from '@douyinfe/semi-ui';
import { API, showError, showSuccess, showWarning } from '../../../helpers';
import { useTranslation } from 'react-i18next';

function parseChannelIDs(values) {
  if (!Array.isArray(values) || values.length === 0) {
    return [];
  }
  const seen = new Set();
  const result = [];
  values.forEach((item) => {
    String(item)
      .split(/[\s,，]+/)
      .map((part) => part.trim())
      .filter(Boolean)
      .forEach((part) => {
        const id = parseInt(part, 10);
        if (Number.isInteger(id) && id > 0 && !seen.has(id)) {
          seen.add(id);
          result.push(id);
        }
      });
  });
  return result;
}

export default function SettingsTTFTMonitoring() {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const refForm = useRef();
  const [inputs, setInputs] = useState({
    ttft_monitor_enabled: false,
    ttft_monitor_channel_ids: [],
    ttft_monitor_threshold_seconds: 60,
    ttft_monitor_window_seconds: 60,
    ttft_monitor_count_threshold: 10,
    ttft_monitor_cooldown_seconds: 300,
    ttft_monitor_notify_type: 'root_notify',
    ttft_monitor_webhook_url: '',
    ttft_monitor_webhook_secret: '',
  });
  const [inputsRow, setInputsRow] = useState(inputs);

  async function loadConfig() {
    setLoading(true);
    try {
      const res = await API.get('/api/monitor/ttft/config');
      const { success, message, data } = res.data;
      if (!success) {
        throw new Error(message || t('加载 TTFT 监控设置失败'));
      }
      const next = {
        ttft_monitor_enabled: !!data?.enabled,
        ttft_monitor_channel_ids: Array.isArray(data?.channel_ids)
          ? data.channel_ids.map((item) => String(item))
          : [],
        ttft_monitor_threshold_seconds: data?.threshold_seconds || 60,
        ttft_monitor_window_seconds: data?.window_seconds || 60,
        ttft_monitor_count_threshold: data?.count_threshold || 10,
        ttft_monitor_cooldown_seconds: data?.cooldown_seconds || 300,
        ttft_monitor_notify_type: data?.notify_type || 'root_notify',
        ttft_monitor_webhook_url: data?.webhook_url || '',
        ttft_monitor_webhook_secret: '',
      };
      setInputs(next);
      setInputsRow(structuredClone(next));
      if (refForm.current) {
        refForm.current.setValues(next);
      }
      setLoaded(true);
    } catch (error) {
      showError(error.message || t('加载 TTFT 监控设置失败'));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    loadConfig();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function onSubmit() {
    if (!loaded) {
      return showWarning(t('TTFT 监控配置尚未加载完成，请稍后重试'));
    }
    const changed = JSON.stringify(inputs) !== JSON.stringify(inputsRow);
    if (!changed) {
      return showWarning(t('你似乎并没有修改什么'));
    }

    const payload = {
      enabled: !!inputs.ttft_monitor_enabled,
      channel_ids: parseChannelIDs(inputs.ttft_monitor_channel_ids),
      threshold_seconds: parseInt(inputs.ttft_monitor_threshold_seconds, 10) || 60,
      window_seconds: parseInt(inputs.ttft_monitor_window_seconds, 10) || 60,
      count_threshold: parseInt(inputs.ttft_monitor_count_threshold, 10) || 10,
      cooldown_seconds: parseInt(inputs.ttft_monitor_cooldown_seconds, 10) || 300,
      notify_type: inputs.ttft_monitor_notify_type || 'root_notify',
      webhook_url: inputs.ttft_monitor_webhook_url || '',
      webhook_secret: inputs.ttft_monitor_webhook_secret || '',
    };

    setLoading(true);
    try {
      const res = await API.put('/api/monitor/ttft/config', payload);
      if (!res.data.success) {
        throw new Error(res.data.message || t('保存失败，请重试'));
      }
      showSuccess(t('保存成功'));
      await loadConfig();
    } catch (error) {
      showError(error.message || t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  }

  return (
    <Spin spinning={loading}>
      <Form
        values={inputs}
        getFormApi={(formAPI) => (refForm.current = formAPI)}
        style={{ marginBottom: 15 }}
      >
        <Form.Section text={t('TTFT 监控设置')}>
          <Banner
            type='info'
            description={t(
              'TTFT 监控：当指定渠道在窗口期内出现慢首字响应达到阈值次数时触发告警。',
            )}
            style={{ marginBottom: 12 }}
          />

          <Row gutter={16}>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.Switch
                field={'ttft_monitor_enabled'}
                label={t('启用 TTFT 监控')}
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                onChange={(value) =>
                  setInputs({
                    ...inputs,
                    ttft_monitor_enabled: value,
                  })
                }
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.Select
                field={'ttft_monitor_notify_type'}
                label={t('告警方式')}
                onChange={(value) =>
                  setInputs({
                    ...inputs,
                    ttft_monitor_notify_type: value,
                  })
                }
                optionList={[
                  { label: t('使用 root 通知配置'), value: 'root_notify' },
                  { label: t('直接 Webhook'), value: 'webhook' },
                ]}
              />
            </Col>
          </Row>

          <Row gutter={16}>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.InputNumber
                label={t('慢首字阈值')}
                step={1}
                min={1}
                suffix={t('秒')}
                field={'ttft_monitor_threshold_seconds'}
                onChange={(value) =>
                  setInputs({
                    ...inputs,
                    ttft_monitor_threshold_seconds: parseInt(value),
                  })
                }
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.InputNumber
                label={t('统计窗口')}
                step={1}
                min={1}
                suffix={t('秒')}
                field={'ttft_monitor_window_seconds'}
                onChange={(value) =>
                  setInputs({
                    ...inputs,
                    ttft_monitor_window_seconds: parseInt(value),
                  })
                }
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.InputNumber
                label={t('触发次数')}
                step={1}
                min={1}
                field={'ttft_monitor_count_threshold'}
                onChange={(value) =>
                  setInputs({
                    ...inputs,
                    ttft_monitor_count_threshold: parseInt(value),
                  })
                }
              />
            </Col>
          </Row>

          <Row gutter={16}>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.InputNumber
                label={t('告警冷却时间')}
                step={1}
                min={1}
                suffix={t('秒')}
                field={'ttft_monitor_cooldown_seconds'}
                onChange={(value) =>
                  setInputs({
                    ...inputs,
                    ttft_monitor_cooldown_seconds: parseInt(value),
                  })
                }
              />
            </Col>
            <Col xs={24} sm={24} md={16} lg={16} xl={16}>
              <Form.Slot
                label={t('监控渠道 ID')}
                extraText={t('仅监控这里填写的渠道，输入后回车添加')}
              >
                <TagInput
                  value={inputs.ttft_monitor_channel_ids}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ttft_monitor_channel_ids: value,
                    })
                  }
                  placeholder={t('例如 1, 2, 3')}
                />
              </Form.Slot>
            </Col>
          </Row>

          {inputs.ttft_monitor_notify_type === 'webhook' && (
            <Row gutter={16}>
              <Col xs={24} sm={24} md={12} lg={12} xl={12}>
                <Form.Input
                  field={'ttft_monitor_webhook_url'}
                  label={t('Webhook 地址')}
                  placeholder={'https://example.com/webhook'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ttft_monitor_webhook_url: value,
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={24} md={12} lg={12} xl={12}>
                <Form.Input
                  field={'ttft_monitor_webhook_secret'}
                  label={t('Webhook 密钥')}
                  type='password'
                  placeholder={t('可选，用于签名')}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ttft_monitor_webhook_secret: value,
                    })
                  }
                />
              </Col>
            </Row>
          )}

          <Row>
            <Button size='default' onClick={onSubmit}>
              {t('保存 TTFT 监控设置')}
            </Button>
          </Row>
        </Form.Section>
      </Form>
    </Spin>
  );
}
