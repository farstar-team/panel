import { useState } from 'react';
import { useLocation, useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Drawer, Menu } from 'antd';
import type { MenuProps } from 'antd';
import {
  ApiOutlined,
  ApartmentOutlined,
  CloudServerOutlined,
  ClusterOutlined,
  CodeOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  DiscordOutlined,
  ExportOutlined,
  GithubOutlined,
  GlobalOutlined,
  ImportOutlined,
  LogoutOutlined,
  MailOutlined,
  MenuOutlined,
  MessageOutlined,
  MoonOutlined,
  ReadOutlined,
  SafetyOutlined,
  SettingOutlined,
  SunOutlined,
  SwapOutlined,
  TagsOutlined,
  TeamOutlined,
  ToolOutlined,
  ShopOutlined,
} from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import { formatPanelVersion } from '@/lib/panel-version';
import { useTheme } from '@/hooks/useTheme';
import { useAllSettings } from '@/api/queries/useAllSettings';
import './AppSidebar.css';

const DOCS_URL = 'https://github.com/farstar-team/panel/blob/main/docs/FARSTAR_FA.md';
const REPO_URL = 'https://github.com/farstar-team/panel';

export default function AppSidebar() {
  const { t, i18n } = useTranslation();
  const { isDark, isUltra, toggleTheme, toggleUltra } = useTheme();
  const { pathname, hash } = useLocation();
  const navigate = useNavigate();
  const { allSetting } = useAllSettings();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [openKeys, setOpenKeys] = useState<string[]>([]);
  const activeSubmenu = pathname === '/settings' || pathname === '/xray' ? pathname : null;
  const selectedKey = activeSubmenu
    ? `${pathname}${hash || (pathname === '/settings' ? '#general' : '#basic')}`
    : pathname;

  const settingsChildren: NonNullable<MenuProps['items']> = [
    {
      key: '/settings#general',
      icon: <SettingOutlined />,
      label: t('pages.settings.panelSettings'),
    },
    {
      key: '/settings#security',
      icon: <SafetyOutlined />,
      label: t('pages.settings.securitySettings'),
    },
    {
      key: '/settings#telegram',
      icon: <MessageOutlined />,
      label: t('pages.settings.TGBotSettings'),
    },
    { key: '/settings#email', icon: <MailOutlined />, label: t('pages.settings.emailSettings') },
    {
      key: '/settings#discord',
      icon: <DiscordOutlined />,
      label: t('pages.settings.discordSettings'),
    },
    {
      key: '/settings#subscription',
      icon: <CloudServerOutlined />,
      label: t('pages.settings.subSettings'),
    },
  ];
  if (allSetting.subJsonEnable || allSetting.subClashEnable) {
    settingsChildren.push({
      key: '/settings#subscription-formats',
      icon: <CodeOutlined />,
      label: t('menu.subFormats'),
    });
  }
  if (allSetting.subJsonEnable) {
    settingsChildren.push({
      key: '/settings#subscription-balancers',
      icon: <ApartmentOutlined />,
      label: t('pages.settings.subBalancers.menu'),
    });
  }
  const items: MenuProps['items'] = [
    { key: '/', icon: <DashboardOutlined />, label: t('menu.dashboard') },
    {
      type: 'group',
      key: 'customers',
      label: t('menu.clients'),
      children: [
        { key: '/clients', icon: <TeamOutlined />, label: t('menu.clients') },
        { key: '/groups', icon: <TagsOutlined />, label: t('menu.groups') },
        { key: '/store', icon: <ShopOutlined />, label: t('farstarStore.title') },
      ],
    },
    {
      type: 'group',
      key: 'network',
      label: t('menu.nodes'),
      children: [
        { key: '/inbounds', icon: <ImportOutlined />, label: t('menu.inbounds') },
        { key: '/nodes', icon: <ClusterOutlined />, label: t('menu.nodes') },
        { key: '/hosts', icon: <GlobalOutlined />, label: t('menu.hosts') },
        { key: '/outbound', icon: <ExportOutlined />, label: t('menu.outbounds') },
        { key: '/routing', icon: <SwapOutlined />, label: t('menu.routing') },
      ],
    },
    {
      type: 'group',
      key: 'system',
      label: t('menu.settings'),
      children: [
        {
          key: '/settings',
          icon: <SettingOutlined />,
          label: t('menu.settings'),
          children: settingsChildren,
        },
        {
          key: '/xray',
          icon: <ToolOutlined />,
          label: t('menu.xray'),
          children: [
            { key: '/xray#basic', icon: <SettingOutlined />, label: t('pages.xray.basicTemplate') },
            { key: '/xray#balancer', icon: <ClusterOutlined />, label: t('pages.xray.Balancers') },
            { key: '/xray#dns', icon: <DatabaseOutlined />, label: 'DNS' },
            {
              key: '/xray#advanced',
              icon: <CodeOutlined />,
              label: t('pages.xray.advancedTemplate'),
            },
          ],
        },
        { key: '/api-docs', icon: <ApiOutlined />, label: t('menu.apiDocs') },
      ],
    },
  ];
  const menu = (
    <Menu
      mode="inline"
      theme={isDark ? 'dark' : 'light'}
      selectedKeys={[selectedKey]}
      openKeys={Array.from(new Set([...openKeys, ...(activeSubmenu ? [activeSubmenu] : [])]))}
      onOpenChange={setOpenKeys}
      items={items}
      onClick={({ key }) => {
        navigate(key);
        setDrawerOpen(false);
      }}
    />
  );
  const brand = (
    <div className="farstar-brand">
      <span className="farstar-monogram" aria-hidden="true">
        F
      </span>
      <div>
        <strong>FARSTAR</strong>
        <small>CONTROL ROOM</small>
      </div>
    </div>
  );
  const footer = (
    <div className="farstar-nav-footer">
      <div className="farstar-nav-tools">
        <a
          href={DOCS_URL}
          target="_blank"
          rel="noopener noreferrer"
          aria-label={t('menu.docs')}
          title={t('menu.docs')}
        >
          <ReadOutlined />
        </a>
        <button
          type="button"
          aria-label={t('menu.theme')}
          title={t('menu.theme')}
          onClick={() => {
            if (!isDark) toggleTheme();
            else if (!isUltra) toggleUltra();
            else {
              toggleUltra();
              toggleTheme();
            }
          }}
        >
          {isDark ? <MoonOutlined /> : <SunOutlined />}
        </button>
        <button
          type="button"
          aria-label={t('logout')}
          title={t('logout')}
          onClick={async () => {
            await HttpUtil.post('/logout');
            window.location.href = window.X_UI_BASE_PATH || '/';
          }}
        >
          <LogoutOutlined />
        </button>
      </div>
      <div className="farstar-nav-meta">
        <a href={REPO_URL} target="_blank" rel="noopener noreferrer">
          <GithubOutlined /> {formatPanelVersion(window.X_UI_CUR_VER || '')}
        </a>
        <a
          href={`${window.X_UI_BASE_PATH || '/'}panel/sponsors`}
          onClick={(event) => {
            event.preventDefault();
            navigate('/sponsors');
            setDrawerOpen(false);
          }}
        >
          {t('menu.sponsors')}
        </a>
      </div>
    </div>
  );

  return (
    <>
      <aside className="farstar-navigation" aria-label="FARSTAR">
        {brand}
        <div className="farstar-nav-scroll">{menu}</div>
        {footer}
      </aside>
      <button
        className="farstar-mobile-menu"
        type="button"
        aria-label={t('menu.openMenu')}
        onClick={() => setDrawerOpen(true)}
      >
        <MenuOutlined />
      </button>
      <Drawer
        title="FARSTAR"
        placement={i18n.dir() === 'rtl' ? 'right' : 'left'}
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        size="min(88vw, 320px)"
        className="farstar-nav-drawer"
        styles={{ body: { display: 'flex', flexDirection: 'column', padding: 12 } }}
      >
        <div className="farstar-nav-scroll">{menu}</div>
        {footer}
      </Drawer>
    </>
  );
}
