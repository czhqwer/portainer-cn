import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { confirmDatabaseCommand } from './ConfirmDatabaseCommandModal';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: { defaultValue?: string }) =>
      options?.defaultValue || key,
  }),
}));

describe('ConfirmDatabaseCommandModal', () => {
  afterEach(() => {
    document.body
      .querySelectorAll('[data-reach-dialog-overlay]')
      .forEach((element) => element.remove());
    document.body
      .querySelectorAll('[id^="dialog-"]')
      .forEach((element) => element.remove());
  });

  it('shows the impact information and the exact command before execution', async () => {
    const user = userEvent.setup();
    const resultPromise = confirmDatabaseCommand({
      message: 'This statement will affect 2 row(s). Execute it?',
      command: 'DELETE FROM users WHERE id IN (1, 2)',
      rowsAffected: 2,
    });

    expect(await screen.findByText('Confirm database command')).toBeVisible();
    expect(screen.getByText('Affected rows:')).toBeVisible();
    expect(screen.getByText('2')).toBeVisible();
    expect(
      screen.getByText('DELETE FROM users WHERE id IN (1, 2)')
    ).toBeVisible();

    await user.click(screen.getByRole('button', { name: /cancel/i }));
    await expect(resultPromise).resolves.toBe(false);
  });

  it('returns true only after the user chooses to run the command', async () => {
    const user = userEvent.setup();
    const resultPromise = confirmDatabaseCommand({
      message:
        'This command may modify data or database structure. Execute it?',
      command: 'SET cache:enabled true',
    });

    await user.click(await screen.findByRole('button', { name: /^run$/i }));

    await expect(resultPromise).resolves.toBe(true);
  });
});
