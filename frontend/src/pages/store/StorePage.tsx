import { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { FormProvider } from 'react-hook-form';
import {
  Alert,
  Button,
  Card,
  Col,
  Form,
  Input,
  InputNumber,
  Layout,
  Modal,
  Row,
  Space,
  Switch,
  Table,
  Tag,
} from 'antd';
import { EditOutlined, PlusOutlined } from '@ant-design/icons';
import { HttpUtil } from '@/utils';
import { FormField, useZodForm } from '@/components/form/rhf';
import {
  StoreConfigSchema,
  StorePlanFormSchema,
  StoreStateSchema,
  type StoreConfig,
  type StorePlan,
  type StorePlanForm,
} from '@/schemas/store';

const emptyPlan: StorePlanForm = {
  id: 0,
  name: '',
  price: 100000,
  quotaGB: 30,
  days: 30,
  enabled: true,
};
const jsonOptions = { headers: { 'Content-Type': 'application/json' } };

export default function StorePage() {
  const { t } = useTranslation();
  const [planOpen, setPlanOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const config = useZodForm<StoreConfig>(StoreConfigSchema, {
    defaultValues: { id: 1, enabled: false, paymentText: '', supportURL: '' },
  });
  const plan = useZodForm<StorePlanForm>(StorePlanFormSchema, { defaultValues: emptyPlan });
  const state = useQuery({
    queryKey: ['farstar-store'],
    queryFn: async () => {
      const response = await HttpUtil.get('/panel/api/store/state');
      if (!response.success) throw new Error(response.msg);
      return StoreStateSchema.parse(response.obj);
    },
  });
  useEffect(() => {
    if (state.data) config.reset(state.data.config);
  }, [state.data, config]);

  async function saveSettings(values: StoreConfig) {
    setSaving(true);
    setError('');
    try {
      const result = await HttpUtil.post('/panel/api/store/settings', values, jsonOptions);
      if (!result.success) setError(result.msg);
      else await state.refetch();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : t('somethingWentWrong'));
    } finally {
      setSaving(false);
    }
  }
  async function savePlan(values: StorePlanForm) {
    setSaving(true);
    setError('');
    try {
      const { quotaGB, ...fields } = values;
      const result = await HttpUtil.post(
        '/panel/api/store/plan',
        { ...fields, quotaBytes: Math.round(quotaGB * 2 ** 30) },
        jsonOptions,
      );
      if (!result.success) setError(result.msg);
      else {
        setPlanOpen(false);
        await state.refetch();
      }
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : t('somethingWentWrong'));
    } finally {
      setSaving(false);
    }
  }
  function editPlan(record?: StorePlan) {
    plan.reset(record ? { ...record, quotaGB: record.quotaBytes / 2 ** 30 } : emptyPlan);
    setError('');
    setPlanOpen(true);
  }

  return (
    <Layout className="store-page">
      <Layout className="content-shell">
        <Layout.Content className="content-area">
          <Space orientation="vertical" size={20} style={{ display: 'flex' }}>
            <Alert type="info" showIcon title={t('farstarStore.notice')} />
            {(error || state.error) && (
              <Alert type="error" showIcon title={error || state.error?.message} />
            )}
            <Row gutter={[20, 20]}>
              <Col xs={24} xl={8}>
                <Card title={t('farstarStore.title')} loading={state.isLoading}>
                  <FormProvider {...config}>
                    <form onSubmit={config.handleSubmit(saveSettings)}>
                      <Form layout="vertical" component="div">
                        <FormField
                          name="enabled"
                          label={t('farstarStore.sales')}
                          valueProp="checked"
                        >
                          <Switch />
                        </FormField>
                        <FormField name="paymentText" label={t('farstarStore.payment')}>
                          <Input.TextArea rows={6} maxLength={4000} />
                        </FormField>
                        <FormField name="supportURL" label={t('farstarStore.support')}>
                          <Input placeholder="https://t.me/username" dir="ltr" />
                        </FormField>
                        <Button block type="primary" htmlType="submit" loading={saving}>
                          {t('save')}
                        </Button>
                      </Form>
                    </form>
                  </FormProvider>
                </Card>
              </Col>
              <Col xs={24} xl={16}>
                <Card
                  title={t('farstarStore.plans')}
                  extra={
                    <Button type="primary" icon={<PlusOutlined />} onClick={() => editPlan()}>
                      {t('farstarStore.addPlan')}
                    </Button>
                  }
                >
                  <Table<StorePlan>
                    rowKey="id"
                    dataSource={state.data?.plans || []}
                    loading={state.isLoading}
                    pagination={false}
                    scroll={{ x: 640 }}
                    columns={[
                      { title: t('farstarStore.planName'), dataIndex: 'name' },
                      {
                        title: t('farstarStore.price'),
                        dataIndex: 'price',
                        render: (value: number) => value.toLocaleString(),
                      },
                      {
                        title: t('farstarStore.quota'),
                        dataIndex: 'quotaBytes',
                        render: (value: number) => value / 2 ** 30,
                      },
                      { title: t('farstarStore.days'), dataIndex: 'days' },
                      {
                        title: t('status'),
                        dataIndex: 'enabled',
                        render: (value: boolean) => (
                          <Tag color={value ? 'blue' : undefined}>
                            {t(value ? 'enabled' : 'disabled')}
                          </Tag>
                        ),
                      },
                      {
                        title: t('edit'),
                        key: 'edit',
                        render: (_, record) => (
                          <Button
                            icon={<EditOutlined />}
                            aria-label={t('edit')}
                            onClick={() => editPlan(record)}
                          />
                        ),
                      },
                    ]}
                  />
                </Card>
              </Col>
            </Row>
          </Space>
          <Modal
            open={planOpen}
            title={t('farstarStore.plans')}
            onCancel={() => !saving && setPlanOpen(false)}
            onOk={plan.handleSubmit(savePlan)}
            confirmLoading={saving}
            okText={t('save')}
            cancelText={t('cancel')}
          >
            {error && <Alert type="error" title={error} />}
            <FormProvider {...plan}>
              <Form layout="vertical" component="div">
                <FormField name="name" label={t('farstarStore.planName')}>
                  <Input maxLength={150} />
                </FormField>
                <FormField name="price" label={t('farstarStore.price')}>
                  <InputNumber min={1} max={1_000_000_000_000} style={{ width: '100%' }} />
                </FormField>
                <Row gutter={16}>
                  <Col span={12}>
                    <FormField name="quotaGB" label={t('farstarStore.quota')}>
                      <InputNumber min={0.001} max={100000} style={{ width: '100%' }} />
                    </FormField>
                  </Col>
                  <Col span={12}>
                    <FormField name="days" label={t('farstarStore.days')}>
                      <InputNumber min={1} max={3650} style={{ width: '100%' }} />
                    </FormField>
                  </Col>
                </Row>
                <FormField name="enabled" label={t('enable')} valueProp="checked">
                  <Switch />
                </FormField>
              </Form>
            </FormProvider>
          </Modal>
        </Layout.Content>
      </Layout>
    </Layout>
  );
}
