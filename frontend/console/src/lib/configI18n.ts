// Resolves a config field's label/description/section name in the current
// language (#1241). A ko override comes from i18n/sections/config.ts's
// `fields` map; anything missing (including every `en` entry, by design)
// falls back to the server's own English text, which is already correct
// English copy — see that file's header comment.
import type { ConfigTranslations } from '../i18n/sections/config.ts'
import type { ConfigFieldMeta } from './types'
import type { ConfigFieldLocalization } from './configSearch.ts'

export function localizeConfigField(field: ConfigFieldMeta, text: ConfigTranslations): ConfigFieldLocalization {
  const override = text.fields[field.key]
  return {
    label: override?.label ?? field.label,
    description: override?.description ?? field.description,
    sectionLabel: text.sections[field.section] ?? field.section,
  }
}
