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

import React, { useMemo, useState } from 'react';
import { Banner, Button, Col, Form, Input, Modal, Row, Select, Space, Spin, Switch, Table, Tag, Typography } from '@douyinfe/semi-ui';
import { API, showError, showSuccess, showWarning } from '../../../helpers';
import { CHANNEL_OPTIONS } from '../../../constants/channel.constants';
import { useTranslation } from 'react-i18next';

const defaultForm = {
  id: 0,
  group_name: '',
  channel_type: undefined,
  fallback_channel_id: undefined,
  enabled: true,
  remark: '',
};

export default function SettingsGroupChannelFallback() {
  const { t } = useTranslation();
  const { Text } = Typography;
  const [loading, setLoading] = useState(false);
  const [configs, setConfigs] = useState([]);
  const [groups, setGroups] = useState([]);
  const [channels, setChannels] = useState([]);
  const [form, setForm] = useState(defaultForm);
  const [editingRecord, setEditingRecord] = useState(null);
  const [showModal, setShowModal] = useState(false);

  const channelTypeOptions = useMemo(() => {
    return CHANNEL_OPTIONS.map((item) => ({
      value: item.value,
      label: item.label,
    }));
  }, []);

  const fallbackChannelOptions = useMemo(() => {
    const selectedType = Number(form.channel_type);
    return channels
      .filter((ch) => {
        return Number.isInteger(selectedType) && ch.type === selectedType;
      })
      .map((ch) => ({
        value: ch.id,
        label: `${ch.name} (${String(ch.group || '').trim() || '-'})`,
      }));
  }, [channels, form.channel_type]);

  const channelMap = useMemo(() => {
    const map = new Map();
    channels.forEach((item) => {
      map.set(item.id, item);
    });
    return map;
  }, [channels]);

  const selectedFallbackChannel = useMemo(() => {
    const id = Number(form.fallback_channel_id);
    if (!Number.isInteger(id) || id <= 0) {
      return null;
    }
    return channelMap.get(id) || null;
  }, [channelMap, form.fallback_channel_id]);

  const canSubmit = useMemo(() => {
    return (
      String(form.group_name || '').trim() &&
      Number.isInteger(Number(form.channel_type)) &&
      Number(form.channel_type) >= 0 &&
      Number.isInteger(Number(form.fallback_channel_id)) &&
      Number(form.fallback_channel_id) > 0
    );
  }, [form.group_name, form.channel_type, form.fallback_channel_id]);

  async function loadAll() {
    setLoading(true);
    try {
      const [cfgRes, groupRes, channelRes] = await Promise.all([
        API.get('/api/channel/group_fallback'),
        API.get('/api/group/'),
        API.get('/api/channel/', { params: { p: 0, page_size: 1000 } }),
      ]);

      if (!cfgRes?.data?.success) {
        throw new Error(cfgRes?.data?.message || t('加载渠道兜底配置失败'));
      }
      if (!groupRes?.data?.success) {
        throw new Error(groupRes?.data?.message || t('加载分组失败'));
      }
      if (!channelRes?.data?.success) {
        throw new Error(channelRes?.data?.message || t('加载渠道失败'));
      }

      setConfigs(Array.isArray(cfgRes.data.data) ? cfgRes.data.data : []);
      setGroups(Array.isArray(groupRes.data.data) ? groupRes.data.data : []);
      setChannels(Array.isArray(channelRes?.data?.data?.items) ? channelRes.data.data.items : []);
    } catch (error) {
      showError(error.message || t('加载渠道兜底配置失败'));
    } finally {
      setLoading(false);
    }
  }

  React.useEffect(() => {
    loadAll();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function resetForm() {
    setForm(defaultForm);
    setEditingRecord(null);
  }

  function openCreateModal() {
    resetForm();
    setShowModal(true);
  }

  function closeModal() {
    setShowModal(false);
    resetForm();
  }

  async function submitForm() {
    const payload = {
      id: Number(form.id || 0),
      group_name: String(form.group_name || '').trim(),
      channel_type: Number(form.channel_type),
      fallback_channel_id: Number(form.fallback_channel_id),
      enabled: !!form.enabled,
      remark: String(form.remark || '').trim(),
    };
    if (!payload.group_name) {
      return showWarning(t('请先选择分组'));
    }
    if (!Number.isInteger(payload.channel_type) || payload.channel_type < 0) {
      return showWarning(t('请先选择渠道类型'));
    }
    if (!Number.isInteger(payload.fallback_channel_id) || payload.fallback_channel_id <= 0) {
      return showWarning(t('请先选择兜底渠道'));
    }

    setLoading(true);
    try {
      const res = await API.put('/api/channel/group_fallback', payload);
      if (!res?.data?.success) {
        throw new Error(res?.data?.message || t('保存失败，请重试'));
      }
      showSuccess(t('保存成功'));
      closeModal();
      await loadAll();
    } catch (error) {
      showError(error.message || t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  }

  async function removeConfig(record) {
    const payload = {
      group_name: record.group_name,
      channel_type: record.channel_type,
    };
    setLoading(true);
    try {
      const res = await API.delete('/api/channel/group_fallback', { data: payload });
      if (!res?.data?.success) {
        throw new Error(res?.data?.message || t('删除失败，请重试'));
      }
      showSuccess(t('删除成功'));
      await loadAll();
    } catch (error) {
      showError(error.message || t('删除失败，请重试'));
    } finally {
      setLoading(false);
    }
  }

  function fillForEdit(record) {
    setForm({
      id: record.id || 0,
      group_name: record.group_name || '',
      channel_type: record.channel_type,
      fallback_channel_id: record.fallback_channel_id,
      enabled: !!record.enabled,
      remark: record.remark || '',
    });
    setEditingRecord(record);
    setShowModal(true);
  }

  async function toggleEnabled(record, checked) {
    setLoading(true);
    try {
      const res = await API.put('/api/channel/group_fallback', {
        id: Number(record.id || 0),
        group_name: record.group_name,
        channel_type: record.channel_type,
        fallback_channel_id: record.fallback_channel_id,
        enabled: checked,
        remark: record.remark || '',
      });
      if (!res?.data?.success) {
        throw new Error(res?.data?.message || t('更新状态失败，请重试'));
      }
      showSuccess(t('更新成功'));
      await loadAll();
    } catch (error) {
      showError(error.message || t('更新状态失败，请重试'));
    } finally {
      setLoading(false);
    }
  }

  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 80,
    },
    {
      title: t('分组'),
      dataIndex: 'group_name',
    },
    {
      title: t('渠道类型'),
      dataIndex: 'channel_type',
      render: (value) =>
        channelTypeOptions.find((item) => item.value === value)?.label || value,
    },
    {
      title: t('兜底渠道'),
      dataIndex: 'fallback_channel_id',
      render: (value) => {
        const channel = channelMap.get(value);
        if (!channel) {
          return '-';
        }
        return `${channel.name} (${String(channel.group || '-').trim() || '-'})`;
      },
    },
    {
      title: t('启用'),
      dataIndex: 'enabled',
      render: (value, record) => (
        <Switch
          checked={!!value}
          onChange={(checked) => toggleEnabled(record, checked)}
          checkedText='｜'
          uncheckedText='〇'
          size='small'
        />
      ),
      width: 120,
    },
    {
      title: t('备注'),
      dataIndex: 'remark',
      render: (value) => value || '-',
    },
    {
      title: t('操作'),
      dataIndex: 'operate',
      width: 180,
      render: (_, record) => (
        <Space>
          <Button size='small' onClick={() => fillForEdit(record)}>
            {t('编辑')}
          </Button>
          <Button size='small' type='danger' onClick={() => removeConfig(record)}>
            {t('删除')}
          </Button>
        </Space>
      ),
    },
  ];

  return (
    <Spin spinning={loading}>
      <Form style={{ marginBottom: 15 }}>
        <Form.Section text={t('分组渠道兜底配置')}>
          <Banner
            type='info'
            description={t(
              '该配置仅在主重试次数耗尽且请求仍失败时生效。系统会按“分组 + 渠道类型”匹配，并仅额外尝试一次兜底渠道。',
            )}
            style={{ marginBottom: 12 }}
          />
          <Space>
            <Button onClick={openCreateModal}>{t('新增分组兜底配置')}</Button>
          </Space>
        </Form.Section>
      </Form>

      <Table
        rowKey='id'
        size='small'
        pagination={false}
        dataSource={configs}
        columns={columns}
        empty={t('暂无分组兜底配置')}
      />

      <Modal
        title={editingRecord ? t('编辑分组兜底配置') : t('新增分组兜底配置')}
        visible={showModal}
        onCancel={closeModal}
        onOk={submitForm}
        okText={t('保存')}
        cancelText={t('取消')}
        width={760}
        centered
        okButtonProps={{ disabled: !canSubmit, loading }}
        cancelButtonProps={{ disabled: loading }}
        closeOnEsc
        maskClosable={false}
      >
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 14,
          }}
        >
          <Banner
            type='info'
            bordered
            description={t(
              '选择分组与渠道类型后，可指定任意同类型渠道作为兜底渠道。保存后仅在主重试耗尽时触发一次。',
            )}
          />
        </div>
        <div style={{ marginTop: 14 }}>
          <Row gutter={16}>
            <Col span={12}>
              <Text strong style={{ display: 'block', marginBottom: 8 }}>
                {t('分组')}
              </Text>
              <Select
                value={form.group_name}
                placeholder={t('请选择分组')}
                filter
                style={{ width: '100%' }}
                disabled={!!editingRecord}
                optionList={groups.map((group) => ({
                  label: group,
                  value: group,
                }))}
                onChange={(value) => {
                  setForm((prev) => ({
                    ...prev,
                    group_name: value,
                  }));
                }}
              />
            </Col>
            <Col span={12}>
              <Text strong style={{ display: 'block', marginBottom: 8 }}>
                {t('渠道类型')}
              </Text>
              <Select
                value={form.channel_type}
                placeholder={t('请选择渠道类型')}
                filter
                style={{ width: '100%' }}
                disabled={!!editingRecord}
                optionList={channelTypeOptions}
                onChange={(value) => {
                  setForm((prev) => ({
                    ...prev,
                    channel_type: value,
                    fallback_channel_id: undefined,
                  }));
                }}
              />
            </Col>
          </Row>

          <Row gutter={16} style={{ marginTop: 14 }}>
            <Col span={24}>
              <Text strong style={{ display: 'block', marginBottom: 8 }}>
                {t('兜底渠道')}
              </Text>
              <Select
                value={form.fallback_channel_id}
                optionList={fallbackChannelOptions}
                filter
                style={{ width: '100%' }}
                placeholder={t('请先选择渠道类型')}
                onChange={(value) => {
                  setForm((prev) => ({
                    ...prev,
                    fallback_channel_id: value,
                  }));
                }}
              />
              <Text type='tertiary' size='small' style={{ marginTop: 6, display: 'block' }}>
                {t('可选范围：该渠道类型下任意渠道（不限制分组）')}
              </Text>
            </Col>
          </Row>

          {selectedFallbackChannel && (
            <div
              style={{
                marginBottom: 10,
                padding: '8px 10px',
                borderRadius: 6,
                background: 'var(--semi-color-fill-0)',
                border: '1px solid var(--semi-color-border)',
              }}
            >
              <Space spacing={8}>
                <Text type='secondary' size='small'>
                  {t('已选择')}
                </Text>
                <Tag color='blue'>{selectedFallbackChannel.name}</Tag>
                <Tag color='white'>{`${t('分组')}: ${String(selectedFallbackChannel.group || '-').trim() || '-'}`}</Tag>
              </Space>
            </div>
          )}

          <Row gutter={16} style={{ marginTop: 14 }}>
            <Col span={10}>
              <div
                style={{
                  marginTop: 20,
                  padding: '10px 12px',
                  borderRadius: 6,
                  border: '1px solid var(--semi-color-border)',
                }}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <Text strong>{t('启用规则')}</Text>
                  <Switch
                    checked={!!form.enabled}
                    checkedText='｜'
                    uncheckedText='〇'
                    onChange={(value) =>
                      setForm((prev) => ({
                        ...prev,
                        enabled: value,
                      }))
                    }
                  />
                </div>
                <Text type='tertiary' size='small'>
                  {t('关闭后保留配置但不参与兜底。')}
                </Text>
              </div>
            </Col>
            <Col span={14}>
              <Text strong style={{ display: 'block', marginBottom: 8 }}>
                {t('备注')}
              </Text>
              <Input
                value={form.remark}
                maxLength={255}
                showClear
                placeholder={t('可选，便于备注该规则用途')}
                onChange={(value) => {
                  setForm((prev) => ({
                    ...prev,
                    remark: value,
                  }));
                }}
              />
            </Col>
          </Row>
        </div>
      </Modal>
    </Spin>
  );
}
