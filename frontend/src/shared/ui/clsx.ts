/** Joins truthy class names. */
export default function clsx(...names: Array<string | false | null | undefined>): string {
  return names.filter(Boolean).join(' ');
}
