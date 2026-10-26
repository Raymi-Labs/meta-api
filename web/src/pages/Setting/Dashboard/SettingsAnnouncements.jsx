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

import React, { useEffect, useState, useRef, useReducer } from 'react';
import {
  Button,
  Space,
  Table,
  Form,
  Typography,
  Empty,
  Divider,
  Modal,
  Tag,
  Switch,
  TextArea,
  Tooltip,
  Checkbox,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { Plus, Edit, Trash2, Save, Bell, Maximize2 } from 'lucide-react';
import {
  API,
  showError,
  showSuccess,
  getRelativeTime,
  formatDateTimeString,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';
import AnnouncementEmailHistory from './AnnouncementEmailHistory';
import {
  announcementDraftReducer,
  createAnnouncementID,
  parseAnnouncementDraft,
  publishAnnouncements,
} from '../../../helpers/announcementEmail';

const { Text } = Typography;

const ANNOUNCEMENT_CONTENT_LIMIT = 500;
const ANNOUNCEMENT_EXTRA_LIMIT = 200;

const getCharacterCount = (value = '') => Array.from(value || '').length;

const SettingsAnnouncements = ({ options, refresh }) => {
  const { t } = useTranslation();

  const [draft, dispatchDraft] = useReducer(announcementDraftReducer, {
    items: [],
    dirty: false,
    revision: 0,
  });
  const announcementsList = draft.items;
  const hasChanges = draft.dirty;
  const setAnnouncementsList = (items) =>
    dispatchDraft({ type: 'edit', items });
  const [emailSync, setEmailSync] = useState(true);
  const [emailHistoryRefresh, setEmailHistoryRefresh] = useState(0);
  const [showAnnouncementModal, setShowAnnouncementModal] = useState(false);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [showContentModal, setShowContentModal] = useState(false);
  const [deletingAnnouncement, setDeletingAnnouncement] = useState(null);
  const [editingAnnouncement, setEditingAnnouncement] = useState(null);
  const [modalLoading, setModalLoading] = useState(false);
  const [loading, setLoading] = useState(false);
  const [announcementForm, setAnnouncementForm] = useState({
    content: '',
    publishDate: new Date(),
    type: 'default',
    extra: '',
  });
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [selectedRowKeys, setSelectedRowKeys] = useState([]);

  // 面板启用状态
  const [panelEnabled, setPanelEnabled] = useState(true);

  const formApiRef = useRef(null);
  const contentCharacterCount = getCharacterCount(announcementForm.content);
  const extraCharacterCount = getCharacterCount(announcementForm.extra);

  const contentCountText = `按字符数计算：${contentCharacterCount}/${ANNOUNCEMENT_CONTENT_LIMIT}`;
  const extraCountText = `按字符数计算：${extraCharacterCount}/${ANNOUNCEMENT_EXTRA_LIMIT}`;

  const validateAnnouncementForm = () => {
    if (!announcementForm.content || !announcementForm.publishDate) {
      showError('请填写完整的公告信息');
      return false;
    }

    if (contentCharacterCount > ANNOUNCEMENT_CONTENT_LIMIT) {
      showError(`公告内容不能超过${ANNOUNCEMENT_CONTENT_LIMIT}个字符`);
      return false;
    }

    if (extraCharacterCount > ANNOUNCEMENT_EXTRA_LIMIT) {
      showError(`说明信息不能超过${ANNOUNCEMENT_EXTRA_LIMIT}个字符`);
      return false;
    }

    return true;
  };

  const typeOptions = [
    { value: 'default', label: t('默认') },
    { value: 'ongoing', label: t('进行中') },
    { value: 'success', label: t('成功') },
    { value: 'warning', label: t('警告') },
    { value: 'error', label: t('错误') },
  ];

  const getTypeColor = (type) => {
    const colorMap = {
      default: 'grey',
      ongoing: 'blue',
      success: 'green',
      warning: 'orange',
      error: 'red',
    };
    return colorMap[type] || 'grey';
  };

  const columns = [
    {
      title: t('内容'),
      dataIndex: 'content',
      key: 'content',
      render: (text) => (
        <Tooltip content={text} position='topLeft' showArrow>
          <div
            style={{
              maxWidth: '300px',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {text}
          </div>
        </Tooltip>
      ),
    },
    {
      title: t('发布时间'),
      dataIndex: 'publishDate',
      key: 'publishDate',
      width: 180,
      render: (publishDate) => (
        <div>
          <div style={{ fontWeight: 'bold' }}>
            {getRelativeTime(publishDate)}
          </div>
          <div
            style={{
              fontSize: '12px',
              color: 'var(--semi-color-text-2)',
              marginTop: '2px',
            }}
          >
            {publishDate ? formatDateTimeString(new Date(publishDate)) : '-'}
          </div>
        </div>
      ),
    },
    {
      title: t('类型'),
      dataIndex: 'type',
      key: 'type',
      width: 100,
      render: (type) => (
        <Tag color={getTypeColor(type)} shape='circle'>
          {typeOptions.find((opt) => opt.value === type)?.label || type}
        </Tag>
      ),
    },
    {
      title: t('说明'),
      dataIndex: 'extra',
      key: 'extra',
      render: (text) => (
        <Tooltip content={text || '-'} showArrow>
          <div
            style={{
              maxWidth: '200px',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              color: 'var(--semi-color-text-2)',
            }}
          >
            {text || '-'}
          </div>
        </Tooltip>
      ),
    },
    {
      title: t('操作'),
      key: 'action',
      fixed: 'right',
      width: 150,
      render: (text, record) => (
        <Space>
          <Button
            icon={<Edit size={14} />}
            theme='light'
            type='tertiary'
            size='small'
            disabled={loading}
            onClick={() => handleEditAnnouncement(record)}
          >
            {t('编辑')}
          </Button>
          <Button
            icon={<Trash2 size={14} />}
            type='danger'
            theme='light'
            size='small'
            disabled={loading}
            onClick={() => handleDeleteAnnouncement(record)}
          >
            {t('删除')}
          </Button>
        </Space>
      ),
    },
  ];

  const submitAnnouncements = async () => {
    const revision = draft.revision;
    try {
      setLoading(true);
      const queued = await publishAnnouncements(
        API,
        announcementsList,
        emailSync,
        t('系统公告更新失败'),
      );
      dispatchDraft({ type: 'saved', revision });
      setEmailHistoryRefresh((value) => value + 1);
      showSuccess(
        queued > 0
          ? t('公告已保存，{{count}} 条公告正在后台发送邮件', { count: queued })
          : t('系统公告已更新'),
      );
      // A settings refresh must not turn a successful publication into a failure.
      Promise.resolve(refresh?.()).catch(() => {});
    } catch (error) {
      showError(error.message || t('系统公告更新失败'));
    } finally {
      setLoading(false);
    }
  };

  const handleAddAnnouncement = () => {
    setEditingAnnouncement(null);
    setAnnouncementForm({
      content: '',
      publishDate: new Date(),
      type: 'default',
      extra: '',
    });
    setShowAnnouncementModal(true);
  };

  const handleEditAnnouncement = (announcement) => {
    setEditingAnnouncement(announcement);
    setAnnouncementForm({
      content: announcement.content,
      publishDate: announcement.publishDate
        ? new Date(announcement.publishDate)
        : new Date(),
      type: announcement.type || 'default',
      extra: announcement.extra || '',
    });
    setShowAnnouncementModal(true);
  };

  const handleDeleteAnnouncement = (announcement) => {
    setDeletingAnnouncement(announcement);
    setShowDeleteModal(true);
  };

  const confirmDeleteAnnouncement = () => {
    if (deletingAnnouncement) {
      const newList = announcementsList.filter(
        (item) => item.id !== deletingAnnouncement.id,
      );
      setAnnouncementsList(newList);
      showSuccess('公告已删除，请及时点击“保存设置”进行保存');
    }
    setShowDeleteModal(false);
    setDeletingAnnouncement(null);
  };

  const handleSaveAnnouncement = async () => {
    if (!validateAnnouncementForm()) {
      return;
    }

    try {
      setModalLoading(true);

      // 将publishDate转换为ISO字符串保存
      const formData = {
        ...announcementForm,
        publishDate: announcementForm.publishDate.toISOString(),
      };

      let newList;
      if (editingAnnouncement) {
        newList = announcementsList.map((item) =>
          item.id === editingAnnouncement.id ? { ...item, ...formData } : item,
        );
      } else {
        const newId = createAnnouncementID();
        const newAnnouncement = {
          id: newId,
          ...formData,
        };
        newList = [...announcementsList, newAnnouncement];
      }

      setAnnouncementsList(newList);
      setShowAnnouncementModal(false);
      showSuccess(
        editingAnnouncement
          ? '公告已更新，请及时点击“保存设置”进行保存'
          : '公告已添加，请及时点击“保存设置”进行保存',
      );
    } catch (error) {
      showError('操作失败: ' + error.message);
    } finally {
      setModalLoading(false);
    }
  };

  useEffect(() => {
    const annStr =
      options['console_setting.announcements'] ?? options.Announcements;
    if (annStr !== undefined) {
      try {
        dispatchDraft({
          type: 'receive',
          items: parseAnnouncementDraft(annStr),
        });
      } catch {
        showError(t('系统公告加载失败，已保留当前内容'));
      }
    }
  }, [options['console_setting.announcements'], options.Announcements, t]);

  useEffect(() => {
    const enabledStr = options['console_setting.announcements_enabled'];
    setPanelEnabled(
      enabledStr === undefined
        ? true
        : enabledStr === 'true' || enabledStr === true,
    );
  }, [options['console_setting.announcements_enabled']]);

  const handleToggleEnabled = async (checked) => {
    const newValue = checked ? 'true' : 'false';
    try {
      const res = await API.put('/api/option/', {
        key: 'console_setting.announcements_enabled',
        value: newValue,
      });
      if (res.data.success) {
        setPanelEnabled(checked);
        showSuccess(t('设置已保存'));
        refresh?.();
      } else {
        showError(res.data.message);
      }
    } catch (err) {
      showError(err.message);
    }
  };

  const handleBatchDelete = () => {
    if (selectedRowKeys.length === 0) {
      showError('请先选择要删除的系统公告');
      return;
    }

    const newList = announcementsList.filter(
      (item) => !selectedRowKeys.includes(item.id),
    );
    setAnnouncementsList(newList);
    setSelectedRowKeys([]);
    showSuccess(
      `已删除 ${selectedRowKeys.length} 个系统公告，请及时点击“保存设置”进行保存`,
    );
  };

  const renderHeader = () => (
    <div className='flex flex-col w-full'>
      <div className='mb-2'>
        <div className='flex items-center text-blue-500'>
          <Bell size={16} className='mr-2' />
          <Text>
            {t(
              '系统公告管理，可以发布系统通知和重要消息（最多100个，前端显示最新20条）',
            )}
          </Text>
        </div>
      </div>

      <div className='my-3'>
        <Checkbox
          id='announcement-email-sync'
          checked={emailSync}
          disabled={loading}
          onChange={(event) => setEmailSync(event.target.checked)}
        >
          {t('发布新公告时同步邮件')}
        </Checkbox>
        <p className='mt-2 text-sm text-[var(--semi-color-text-2)]'>
          {t(
            '点击保存设置后，新增公告将发给已启用且绑定有效邮箱的用户。编辑、删除或重复保存不会重发。',
          )}
        </p>
      </div>

      <Divider margin='12px' />

      <div className='flex flex-col md:flex-row justify-between items-center gap-4 w-full'>
        <div className='flex gap-2 w-full md:w-auto order-2 md:order-1'>
          <Button
            theme='light'
            type='primary'
            icon={<Plus size={14} />}
            className='w-full md:w-auto'
            disabled={loading}
            onClick={handleAddAnnouncement}
          >
            {t('添加公告')}
          </Button>
          <Button
            icon={<Trash2 size={14} />}
            type='danger'
            theme='light'
            onClick={handleBatchDelete}
            disabled={loading || selectedRowKeys.length === 0}
            className='w-full md:w-auto'
          >
            {t('批量删除')}{' '}
            {selectedRowKeys.length > 0 && `(${selectedRowKeys.length})`}
          </Button>
          <Button
            icon={<Save size={14} />}
            onClick={submitAnnouncements}
            loading={loading}
            disabled={!hasChanges || loading}
            type='secondary'
            className='w-full md:w-auto'
          >
            {t('保存设置')}
          </Button>
        </div>

        {/* 启用开关 */}
        <div className='order-1 md:order-2 flex items-center gap-2'>
          <Switch
            checked={panelEnabled}
            onChange={handleToggleEnabled}
            disabled={loading}
          />
          <Text>{panelEnabled ? t('已启用') : t('已禁用')}</Text>
        </div>
      </div>
    </div>
  );

  // 计算当前页显示的数据（按发布时间倒序排序，最新优先显示）
  const getCurrentPageData = () => {
    const sortedList = [...announcementsList].sort((a, b) => {
      const dateA = new Date(a.publishDate).getTime();
      const dateB = new Date(b.publishDate).getTime();
      return dateB - dateA; // 倒序，最新的排在前面
    });

    const startIndex = (currentPage - 1) * pageSize;
    const endIndex = startIndex + pageSize;
    return sortedList.slice(startIndex, endIndex);
  };

  const rowSelection = {
    selectedRowKeys,
    onChange: (selectedRowKeys, selectedRows) => {
      setSelectedRowKeys(selectedRowKeys);
    },
    onSelect: (record, selected, selectedRows) => {
      console.log(`选择行: ${selected}`, record);
    },
    onSelectAll: (selected, selectedRows) => {
      console.log(`全选: ${selected}`, selectedRows);
    },
    getCheckboxProps: (record) => ({
      disabled: false,
      name: record.id,
    }),
  };

  return (
    <>
      <div data-testid='announcement-settings'>
        <Form.Section text={renderHeader()}>
          <Table
            columns={columns}
            dataSource={getCurrentPageData()}
            rowSelection={rowSelection}
            rowKey='id'
            scroll={{ x: 'max-content' }}
            pagination={{
              currentPage: currentPage,
              pageSize: pageSize,
              total: announcementsList.length,
              showSizeChanger: true,
              showQuickJumper: true,
              pageSizeOptions: ['5', '10', '20', '50'],
              onChange: (page, size) => {
                setCurrentPage(page);
                setPageSize(size);
              },
              onShowSizeChange: (current, size) => {
                setCurrentPage(1);
                setPageSize(size);
              },
            }}
            size='middle'
            loading={loading}
            empty={
              <Empty
                image={
                  <IllustrationNoResult style={{ width: 150, height: 150 }} />
                }
                darkModeImage={
                  <IllustrationNoResultDark
                    style={{ width: 150, height: 150 }}
                  />
                }
                description={t('暂无系统公告')}
                style={{ padding: 30 }}
              />
            }
            className='overflow-hidden'
          />
        </Form.Section>

        <AnnouncementEmailHistory refreshKey={emailHistoryRefresh} />
      </div>

      <Modal
        title={editingAnnouncement ? t('编辑公告') : t('添加公告')}
        visible={showAnnouncementModal}
        onOk={handleSaveAnnouncement}
        onCancel={() => setShowAnnouncementModal(false)}
        okText={t('保存')}
        cancelText={t('取消')}
        confirmLoading={modalLoading}
      >
        <Form
          layout='vertical'
          initValues={announcementForm}
          key={editingAnnouncement ? editingAnnouncement.id : 'new'}
          getFormApi={(api) => (formApiRef.current = api)}
        >
          <Form.TextArea
            field='content'
            label={t('公告内容')}
            placeholder={t('请输入公告内容（支持 Markdown/HTML）')}
            extraText={contentCountText}
            rows={3}
            rules={[{ required: true, message: t('请输入公告内容') }]}
            onChange={(value) =>
              setAnnouncementForm({ ...announcementForm, content: value })
            }
          />
          <Button
            theme='light'
            type='tertiary'
            size='small'
            icon={<Maximize2 size={14} />}
            style={{ marginBottom: 16 }}
            onClick={() => setShowContentModal(true)}
          >
            {t('放大编辑')}
          </Button>
          <Form.DatePicker
            field='publishDate'
            label={t('发布日期')}
            type='dateTime'
            rules={[{ required: true, message: t('请选择发布日期') }]}
            onChange={(value) =>
              setAnnouncementForm({ ...announcementForm, publishDate: value })
            }
          />
          <Form.Select
            field='type'
            label={t('公告类型')}
            optionList={typeOptions}
            onChange={(value) =>
              setAnnouncementForm({ ...announcementForm, type: value })
            }
          />
          <Form.Input
            field='extra'
            label={t('说明信息')}
            placeholder={t('可选，公告的补充说明')}
            extraText={extraCountText}
            onChange={(value) =>
              setAnnouncementForm({ ...announcementForm, extra: value })
            }
          />
        </Form>
      </Modal>

      <Modal
        title={t('确认删除')}
        visible={showDeleteModal}
        onOk={confirmDeleteAnnouncement}
        onCancel={() => {
          setShowDeleteModal(false);
          setDeletingAnnouncement(null);
        }}
        okText={t('确认删除')}
        cancelText={t('取消')}
        type='warning'
        okButtonProps={{
          type: 'danger',
          theme: 'solid',
        }}
      >
        <Text>{t('确定要删除此公告吗？')}</Text>
      </Modal>

      {/* 公告内容放大编辑 Modal */}
      <Modal
        title={t('编辑公告内容')}
        visible={showContentModal}
        onOk={() => {
          // 将内容同步到表单
          if (formApiRef.current) {
            formApiRef.current.setValue('content', announcementForm.content);
          }
          setShowContentModal(false);
        }}
        onCancel={() => setShowContentModal(false)}
        okText={t('确定')}
        cancelText={t('取消')}
        width={800}
      >
        <TextArea
          value={announcementForm.content}
          placeholder={t('请输入公告内容（支持 Markdown/HTML）')}
          rows={15}
          style={{ width: '100%' }}
          onChange={(value) =>
            setAnnouncementForm({ ...announcementForm, content: value })
          }
        />
        <div
          style={{
            marginTop: 8,
            textAlign: 'right',
            fontSize: 12,
            color:
              contentCharacterCount > ANNOUNCEMENT_CONTENT_LIMIT
                ? 'var(--semi-color-danger)'
                : 'var(--semi-color-text-2)',
          }}
        >
          {contentCountText}
        </div>
      </Modal>
    </>
  );
};

export default SettingsAnnouncements;
