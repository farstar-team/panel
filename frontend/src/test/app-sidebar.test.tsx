import { act, fireEvent, screen, within } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router';
import { expect, test, vi } from 'vitest';

import AppSidebar from '@/layouts/AppSidebar';
import { renderWithProviders } from './test-utils';

vi.mock('@/api/queries/useAllSettings', () => ({
  useAllSettings: () => ({ allSetting: { subJsonEnable: true } }),
}));

function Location() {
  const location = useLocation();
  return (
    <output aria-label="location">
      {location.pathname}
      {location.hash}
    </output>
  );
}

test('keeps deep security settings reachable from the redesigned navigation', async () => {
  const view = renderWithProviders(
    <MemoryRouter initialEntries={['/settings#general']}>
      <AppSidebar />
      <Location />
    </MemoryRouter>,
  );
  await act(async () => {});
  const navigation = within(view.container.querySelector('aside')!);
  fireEvent.click(navigation.getByText('Authentication'));
  expect(screen.getByLabelText('location').textContent).toBe('/settings#security');
});

test('navigates from the mobile drawer and closes it after choosing a page', async () => {
  renderWithProviders(
    <MemoryRouter>
      <AppSidebar />
      <Location />
    </MemoryRouter>,
  );
  await act(async () => {});
  fireEvent.click(screen.getByRole('button', { name: 'Open menu' }));
  const drawer = await screen.findByRole('dialog');
  fireEvent.click(within(drawer).getByRole('menuitem', { name: 'team Clients' }));
  expect(screen.getByLabelText('location').textContent).toBe('/clients');
  await act(async () => {});
  expect(drawer.closest('.ant-drawer')?.classList.contains('ant-drawer-open')).toBe(false);
});
