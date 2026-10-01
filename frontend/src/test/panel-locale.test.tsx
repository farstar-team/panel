import { useContext } from 'react';
import { act, render } from '@testing-library/react';
import { ConfigProvider } from 'antd';
import i18next from 'i18next';
import { expect, it } from 'vitest';

import PanelLocaleProvider from '@/layouts/PanelLocaleProvider';

function DirectionProbe() {
  const { direction } = useContext(ConfigProvider.ConfigContext);
  return <span>{direction}</span>;
}

it('applies Persian RTL to document and AntD and reacts to language changes', async () => {
  const previous = document.documentElement.getAttribute('dir');
  const previousLang = document.documentElement.getAttribute('lang');
  await i18next.changeLanguage('fa-IR');
  const { getByText, unmount } = render(
    <PanelLocaleProvider>
      <DirectionProbe />
    </PanelLocaleProvider>,
  );
  try {
    expect(document.documentElement.getAttribute('dir')).toBe('rtl');
    expect(document.documentElement.getAttribute('lang')).toBe('fa-IR');
    expect(getByText('rtl')).toBeTruthy();
    await act(() => i18next.changeLanguage('en-US'));
    expect(document.documentElement.getAttribute('dir')).toBe('ltr');
    expect(document.documentElement.getAttribute('lang')).toBe('en-US');
    expect(getByText('ltr')).toBeTruthy();
  } finally {
    unmount();
    await i18next.changeLanguage('en-US');
  }
  expect(document.documentElement.getAttribute('dir')).toBe(previous);
  expect(document.documentElement.getAttribute('lang')).toBe(previousLang);
});
