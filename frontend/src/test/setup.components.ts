import { afterAll, afterEach, vi } from 'vitest';
import { act, cleanup } from '@testing-library/react';
import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';
import { message, notification } from 'antd';
import { actDestroy as resetMessage } from 'antd/es/message';
import { actDestroy as resetNotification } from 'antd/es/notification';
import { unmount } from '@rc-component/util';

import enUS from '../../../internal/web/translation/en-US.json';

// Static AntD notices own detached React roots outside Testing Library's registry.
const detachedContainers = new Set<DocumentFragment>();
const createDocumentFragment = document.createDocumentFragment.bind(document);
document.createDocumentFragment = () => {
  const fragment = createDocumentFragment();
  detachedContainers.add(fragment);
  return fragment;
};

// RTL sets this from a global beforeAll, which never runs with `globals: false`.
(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

vi.mock('persian-calendar-suite', () => ({
  PersianDateTimePicker: () => null,
}));

if (typeof globalThis.localStorage === 'undefined') {
  const store = new Map<string, string>();
  const storage = {
    getItem: (k: string) => (store.has(k) ? store.get(k)! : null),
    setItem: (k: string, v: string) => {
      store.set(k, String(v));
    },
    removeItem: (k: string) => {
      store.delete(k);
    },
    clear: () => {
      store.clear();
    },
    key: (i: number) => Array.from(store.keys())[i] ?? null,
    get length() {
      return store.size;
    },
  } as Storage;
  Object.defineProperty(globalThis, 'localStorage', { value: storage, configurable: true });
  Object.defineProperty(globalThis, 'sessionStorage', { value: storage, configurable: true });
}

if (!window.matchMedia) {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
}

if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
}

if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => {};
}

// jsdom does not implement pseudo-element styles or Range geometry. Ant
// Design and CodeMirror use these APIs for layout, so supply harmless test
// fallbacks instead of emitting noisy "Not implemented" errors.
const nativeGetComputedStyle = window.getComputedStyle.bind(window);
window.getComputedStyle = ((element: Element) =>
  nativeGetComputedStyle(element)) as typeof window.getComputedStyle;

if (!Range.prototype.getClientRects) {
  Range.prototype.getClientRects = () => [] as unknown as DOMRectList;
}

if (!i18next.isInitialized) {
  void i18next.use(initReactI18next).init({
    lng: 'en-US',
    fallbackLng: 'en-US',
    resources: { 'en-US': { translation: enUS } },
    interpolation: { escapeValue: false, prefix: '{', suffix: '}' },
    returnNull: false,
  });
}

afterEach(async () => {
  // Async act drains React's scheduled effects before jsdom destroys window.
  await act(async () => {
    message.destroy();
    notification.destroy();
    cleanup();
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  await act(async () => {
    for (const container of detachedContainers) await unmount(container);
    detachedContainers.clear();
    resetMessage();
    resetNotification();
  });
  for (let i = 0; i < 3; i += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
  document.body.innerHTML = '';
});

afterAll(async () => {
  await act(async () => {
    for (const container of detachedContainers) await unmount(container);
    detachedContainers.clear();
  });
  document.createDocumentFragment = createDocumentFragment;
});

import { HttpUtil, Msg } from '@/utils';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
vi.spyOn(HttpUtil, 'post').mockResolvedValue({ success: true, obj: {} } as any);
vi.spyOn(HttpUtil, 'get').mockImplementation(
  async (url: string) => new Msg(true, '', url.includes('/panel/api/inbounds/options') ? [] : {}),
);
