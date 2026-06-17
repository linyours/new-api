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
import { Banner, Button, Col, Form, Row, Spin } from '@douyinfe/semi-ui';
import { API, showError, showSuccess, showWarning } from '../../../helpers';
import { useTranslation } from 'react-i18next';

export default function SettingsChannelDisableMonitoring() {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const refForm = useRef();
  const [inputs, setInputs] = useState({
    channel_disable_monitor_enabled: false,
    channel_disable_monitor_notify_type: 'root_notify',
    channel_disable_monitor_webhook_url: '',
    channel_disable_monitor_webhook_secret: '',
  });
  const [inputsRow, setInputsRow] = useState(inputs);

  async function loadConfig() {
    setLoading(true);
    try {
      const res = await API.get('/api/monitor/channel_disable/config');
      const { success, message, data } = res.data;
      if (!success) {
        throw new Error(message || t('加载通道自动禁用监控设置失败'));
      }
      const next = {
        channel_disable_monitor_enabled: !!data?.enabled,
        channel_disable_monitor_notify_type: data?.notify_type || 'root_notify',
        channel_disable_monitor_webhook_url: data?.webhook_url || '',
        channel_disable_monitor_webhook_secret: '',
      };
      setInputs(next);
      setInputsRow(structuredClone(next));
      if (refForm.current) {
        refForm.current.setValues(next);
      }
      setLoaded(true);
    } catch (error) {
      showError(error.message || t('加载通道自动禁用监控设置失败'));
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
      return showWarning(t('通道自动禁用监控配置尚未加载完成，请稍后重试'));
    }
    const changed = JSON.stringify(inputs) !== JSON.stringify(inputsRow);
    if (!changed) {
      return showWarning(t('你似乎并没有修改什么'));
    }

    const payload = {
      enabled: !!inputs.channel_disable_monitor_enabled,
      notify_type: inputs.channel_disable_monitor_notify_type || 'root_notify',
      webhook_url: inputs.channel_disable_monitor_webhook_url || '',
      webhook_secret: inputs.channel_disable_monitor_webhook_secret || '',
    };

    setLoading(true);
    try {
      const res = await API.put('/api/monitor/channel_disable/config', payload);
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
        <Form.Section text={t('通道自动禁用告警设置')}>
          <Banner
            type='info'
            description={t(
              '当任意渠道被系统自动禁用时触发告警，可选择使用 root 通知配置或直接发送到 Webhook。',
            )}
            style={{ marginBottom: 12 }}
          />

          <Row gutter={16}>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.Switch
                field={'channel_disable_monitor_enabled'}
                label={t('启用通道自动禁用告警')}
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                onChange={(value) =>
                  setInputs({
                    ...inputs,
                    channel_disable_monitor_enabled: value,
                  })
                }
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.Select
                field={'channel_disable_monitor_notify_type'}
                label={t('告警方式')}
                onChange={(value) =>
                  setInputs({
                    ...inputs,
                    channel_disable_monitor_notify_type: value,
                  })
                }
                optionList={[
                  { label: t('使用 root 通知配置'), value: 'root_notify' },
                  { label: t('直接 Webhook'), value: 'webhook' },
                ]}
              />
            </Col>
          </Row>

          {inputs.channel_disable_monitor_notify_type === 'webhook' && (
            <Row gutter={16}>
              <Col xs={24} sm={24} md={12} lg={12} xl={12}>
                <Form.Input
                  field={'channel_disable_monitor_webhook_url'}
                  label={t('Webhook 地址')}
                  placeholder={'https://example.com/webhook'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      channel_disable_monitor_webhook_url: value,
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={24} md={12} lg={12} xl={12}>
                <Form.Input
                  field={'channel_disable_monitor_webhook_secret'}
                  label={t('Webhook 密钥')}
                  type='password'
                  placeholder={t('可选，用于签名')}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      channel_disable_monitor_webhook_secret: value,
                    })
                  }
                />
              </Col>
            </Row>
          )}

          <Row>
            <Button size='default' onClick={onSubmit}>
              {t('保存通道自动禁用告警设置')}
            </Button>
          </Row>
        </Form.Section>
      </Form>
    </Spin>
  );
}
