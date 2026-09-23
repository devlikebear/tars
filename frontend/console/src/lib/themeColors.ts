// Literal theme colors for libraries that cannot read CSS custom properties
// (xterm.js paints on canvas; Mermaid does color math on its inputs).
// Everything else uses the tokens in app.css. tests/themeColors.test.ts keeps
// these in step with those tokens.

export const themeColors = {
  surfaceBase: '#0d0f11',
  surface: '#14171a',
  surfaceElevated: '#1b1f23',
  surfaceHover: '#21262b',
  surfaceInset: '#0a0c0e',
  borderDefault: '#2b3137',
  borderStrong: '#3a4148',
  textPrimary: '#e6e9ec',
  textSecondary: '#9aa4ad',
  textTertiary: '#6b757e',
  primary: '#3ee07f',
  primaryHover: '#2fc56b',
  primaryContrast: '#06140c',
} as const

// Terminal: graphite background, green cursor, and an ANSI palette tuned
// to the same cool neutrals.
export const terminalTheme = {
  background: themeColors.surfaceInset,
  foreground: themeColors.textPrimary,
  cursor: themeColors.primary,
  cursorAccent: themeColors.primaryContrast,
  selectionBackground: '#1f4d33',
  selectionForeground: '#ffffff',
  black: '#14171a',
  red: '#ff6b6b',
  green: '#3ee07f',
  yellow: '#f5c542',
  blue: '#5cb8ff',
  magenta: '#c792ea',
  cyan: '#3fd4b4',
  white: themeColors.textPrimary,
  brightBlack: '#5b646d',
  brightWhite: '#ffffff',
} as const

// xterm search decorations accept only #RRGGBB.
export const terminalSearchDecorations = {
  matchBackground: '#1f4d33',
  matchBorder: themeColors.primary,
  matchOverviewRuler: themeColors.primary,
  activeMatchBackground: themeColors.primary,
  activeMatchBorder: '#ffffff',
  activeMatchColorOverviewRuler: '#ffffff',
} as const

export const mermaidThemeVariables = {
  darkMode: true,
  background: themeColors.surface,
  // Graphite nodes with green edges: light text stays readable.
  primaryColor: themeColors.surfaceElevated,
  primaryTextColor: themeColors.textPrimary,
  primaryBorderColor: themeColors.primary,
  lineColor: themeColors.textTertiary,
  secondaryColor: themeColors.surfaceHover,
  tertiaryColor: themeColors.surface,
} as const
