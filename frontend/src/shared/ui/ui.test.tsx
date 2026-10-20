import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { passwordLength } from './Field';
import { ReasonDialog } from './ReasonDialog';

describe('passwordLength', () => {
  it('counts Unicode code points like the server', () => {
    expect(passwordLength('пароль')).toBe(6);
    expect(passwordLength('🐶🐶')).toBe(2);
  });
});

describe('ReasonDialog', () => {
  it('requires a trimmed reason and passes it on', async () => {
    const onConfirm = vi.fn(async () => undefined);
    const onClose = vi.fn();
    render(<ReasonDialog open title="Отметить ошибочной" confirmLabel="Отметить" onConfirm={onConfirm} onClose={onClose} />);
    const confirm = screen.getByRole('button', { name: 'Отметить' });
    expect((confirm as HTMLButtonElement).disabled).toBe(true);
    await userEvent.type(screen.getByLabelText('Причина'), '   ');
    expect((confirm as HTMLButtonElement).disabled).toBe(true);
    await userEvent.type(screen.getByLabelText('Причина'), 'Ошибочная запись ');
    await userEvent.click(confirm);
    expect(onConfirm).toHaveBeenCalledWith('Ошибочная запись');
    expect(onClose).toHaveBeenCalled();
  });
});
