import { useLayoutEffect } from 'react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { ConfigProvider } from 'antd';

export default function PanelLocaleProvider({ children }: { children: ReactNode }) {
  const { i18n } = useTranslation();
  const direction = i18n.dir(i18n.language);

  useLayoutEffect(() => {
    const previous = document.documentElement.getAttribute('dir');
    document.documentElement.setAttribute('dir', direction);
    return () => {
      if (previous === null) document.documentElement.removeAttribute('dir');
      else document.documentElement.setAttribute('dir', previous);
    };
  }, [direction]);

  return <ConfigProvider direction={direction}>{children}</ConfigProvider>;
}
