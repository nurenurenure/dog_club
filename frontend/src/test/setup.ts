import '@testing-library/dom';
import { cleanup, configure } from '@testing-library/react';
import { afterEach, vi } from 'vitest';

// jsdom has no modal dialogs; enough to make <dialog> contents visible to queries.
if (typeof HTMLDialogElement !== 'undefined') {
  HTMLDialogElement.prototype.showModal ??= function showModal(this: HTMLDialogElement) {
    this.setAttribute('open', '');
  };
  HTMLDialogElement.prototype.close ??= function close(this: HTMLDialogElement) {
    this.removeAttribute('open');
    this.dispatchEvent(new Event('close'));
  };
}

// findBy… waits 1 s by default; a full parallel run on a busy machine needs more.
configure({ asyncUtilTimeout: 3000 });

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
