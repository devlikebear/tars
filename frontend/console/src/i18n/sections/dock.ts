// Dock zones and panel frames (ChatDockHost, DockPanelFrame).
export const dockEn = {
  dockLeft: 'Dock left',
  dockRight: 'Dock right',
  dockBottom: 'Dock bottom',
  fullscreen: 'Fullscreen',
  closePanel: 'Close panel',
  dockedPanels: 'Docked panels',
  closeTab: (title: string) => `Close ${title}`,
  loading: 'Loading...',
  resizeLeft: 'Resize left dock',
  resizeRight: 'Resize right dock',
  resizeBottom: 'Resize bottom dock',
}

export type DockTranslations = typeof dockEn

export const dockKo: DockTranslations = {
  dockLeft: '왼쪽에 도킹',
  dockRight: '오른쪽에 도킹',
  dockBottom: '아래쪽에 도킹',
  fullscreen: '전체 화면',
  closePanel: '패널 닫기',
  dockedPanels: '도킹된 패널',
  closeTab: (title) => `${title} 닫기`,
  loading: '불러오는 중...',
  resizeLeft: '왼쪽 도크 크기 조절',
  resizeRight: '오른쪽 도크 크기 조절',
  resizeBottom: '아래쪽 도크 크기 조절',
}
