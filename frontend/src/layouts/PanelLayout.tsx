import { Outlet, useLocation } from 'react-router';
import { useTranslation } from 'react-i18next';
import { ConfigProvider, Button } from 'antd';
import { SearchOutlined } from '@ant-design/icons';

import { useWebSocketBridge } from '@/api/websocketBridge';
import { usePageTitle } from '@/hooks/usePageTitle';
import CommandPalette from '@/components/command-palette/CommandPalette';
import { useCommandPalette } from '@/components/command-palette/useCommandPalette';
import { useTheme } from '@/hooks/useTheme';
import AppSidebar from './AppSidebar';

export default function PanelLayout() {
  useWebSocketBridge();
  usePageTitle();
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const { antdThemeConfig } = useTheme();
  const { open } = useCommandPalette();
  const titleKeys: Record<string, string> = {
    '/': 'menu.dashboard',
    '/clients': 'menu.clients',
    '/inbounds': 'menu.inbounds',
    '/groups': 'menu.groups',
    '/nodes': 'menu.nodes',
    '/hosts': 'menu.hosts',
    '/store': 'farstarStore.title',
    '/settings': 'menu.settings',
    '/xray': 'menu.xray',
    '/outbound': 'menu.outbounds',
    '/routing': 'menu.routing',
    '/api-docs': 'menu.apiDocs',
    '/sponsors': 'menu.sponsors',
  };
  return (
    <ConfigProvider theme={antdThemeConfig}>
      <div className="farstar-shell css-var-xui">
        <AppSidebar />
        <main className="farstar-workspace">
          <header className="farstar-topbar">
            <div className="farstar-page-title">
              <small>FARSTAR /</small>
              <h1>{t(titleKeys[pathname] || 'menu.dashboard')}</h1>
            </div>
            <Button
              type="text"
              icon={<SearchOutlined />}
              onClick={open}
              aria-label={t('commandPalette.title')}
            >
              <span className="farstar-search-label">{t('commandPalette.search')}</span>
              <kbd>Ctrl K</kbd>
            </Button>
          </header>
          <Outlet />
        </main>
      </div>
      <CommandPalette />
    </ConfigProvider>
  );
}
