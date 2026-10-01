import { useLayoutEffect } from 'react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { ConfigProvider } from 'antd';

export default function PanelLocaleProvider({ children }: { children: ReactNode }) {
  const { i18n } = useTranslation();
  const language = i18n.language;
  const direction = i18n.dir(language);

  useLayoutEffect(() => {
    const previous = document.documentElement.getAttribute('dir');
    const previousLang = document.documentElement.getAttribute('lang');
    document.documentElement.setAttribute('dir', direction);
    document.documentElement.setAttribute('lang', language);
    return () => {
      if (previous === null) document.documentElement.removeAttribute('dir');
      else document.documentElement.setAttribute('dir', previous);
      if (previousLang === null) document.documentElement.removeAttribute('lang');
      else document.documentElement.setAttribute('lang', previousLang);
    };
  }, [direction, language]);

  return <ConfigProvider direction={direction}>{children}</ConfigProvider>;
}
