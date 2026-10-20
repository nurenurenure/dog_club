import { authFetch } from './client';
import { toApiError } from './problem';

// File routes stream bytes or take multipart bodies, so they bypass the JSON client
// but keep its bearer token and single refresh retry.

async function checked(request: Request): Promise<Response> {
  const response = await authFetch(request);
  if (!response.ok) throw toApiError(response.status, await response.json().catch(() => null));
  return response;
}

/** The name from Content-Disposition (RFC 5987 form first), else the fallback. */
export function attachmentName(header: string | null, fallback: string): string {
  if (!header) return fallback;
  const extended = /filename\*=UTF-8''([^;]+)/i.exec(header);
  if (extended?.[1]) {
    try {
      return decodeURIComponent(extended[1]);
    } catch {
      // Fall through to the plain form.
    }
  }
  const plain = /filename="?([^";]+)"?/i.exec(header);
  return plain?.[1] ?? fallback;
}

/** Downloads an API file as an attachment: the browser never opens it inline. */
export async function downloadFile(path: string, fallbackName: string): Promise<void> {
  const response = await checked(new Request(new URL(path, window.location.origin)));
  const blob = await response.blob();
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = attachmentName(response.headers.get('Content-Disposition'), fallbackName);
  document.body.append(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}

/** Sends a multipart form and returns the parsed JSON answer. */
export async function postForm<T>(path: string, form: FormData): Promise<T> {
  const response = await checked(new Request(new URL(path, window.location.origin), { method: 'POST', body: form }));
  return (await response.json()) as T;
}
