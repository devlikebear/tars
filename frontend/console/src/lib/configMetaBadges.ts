// Per-field badges on /console/config (#1242). The old per-field "default" /
// "requires restart" / "live" badges are gone — every field requires a
// restart today (configLiveApplyFields is empty server-side), so that badge
// said the same thing 181/181 times; the save bar now carries one restart
// notice instead (ConfigSaveBar). A field only calls out "changed" (dirty or
// saved-but-non-default) and, separately, "secret".
import { configValuesEqual } from './configStructured.ts'
import type { ConfigFieldMeta } from './types'

export type ConfigMetaBadgeTone = 'changed' | 'secret'

export type ConfigMetaBadge = {
  label: string
  tone: ConfigMetaBadgeTone
  title: string
}

export interface ConfigMetaBadgeText {
  changed: string
  changedTitleDirty: string
  changedTitleSaved: string
  secret: string
  secretTitle: string
}

export function buildConfigMetaBadges(
  field: ConfigFieldMeta,
  value: unknown,
  isDirty: boolean,
  text: ConfigMetaBadgeText,
): ConfigMetaBadge[] {
  const badges: ConfigMetaBadge[] = []
  const hasDefault = Object.prototype.hasOwnProperty.call(field, 'default_value')
  const changedFromDefault = hasDefault && !configValuesEqual(value, field.default_value)

  if (isDirty) {
    badges.push({ label: text.changed, tone: 'changed', title: text.changedTitleDirty })
  } else if (changedFromDefault) {
    badges.push({ label: text.changed, tone: 'changed', title: text.changedTitleSaved })
  }

  if (field.sensitive) {
    badges.push({ label: text.secret, tone: 'secret', title: text.secretTitle })
  }

  return badges
}
