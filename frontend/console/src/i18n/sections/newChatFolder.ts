// "New chat in a folder" next to the board's New chat and the sidebar's
// + New Chat (NewChatFolderMenu). Paths are content and never go through here.
export const newChatFolderEn = {
  open: 'New chat in a folder…',
  title: 'New chat in a folder',
  recent: 'Recent folders',
  noRecent: 'No folders yet. Type one below.',
  path: 'Folder',
  pathPlaceholder: '~/code/project or an absolute path',
  checking: 'Checking…',
  inRepo: 'Git repository',
  notRepo: 'Not a git repository, so the chat works in the folder itself.',
  notFound: 'That folder does not exist.',
  notAFolder: 'Use an absolute path, or one starting with ~, to a folder.',
  isolate: 'Isolate in a worktree',
  isolateHint: 'The chat works on a branch of its own; its edits reach your checkout only when you apply them.',
  start: 'Start chat',
  starting: 'Starting…',
  cancel: 'Cancel',
  failed: (message: string) => `Could not start the chat: ${message}`,
}

export type NewChatFolderTranslations = typeof newChatFolderEn

export const newChatFolderKo: NewChatFolderTranslations = {
  open: '폴더에서 새 채팅…',
  title: '폴더에서 새 채팅',
  recent: '최근 폴더',
  noRecent: '아직 폴더가 없습니다. 아래에 입력하세요.',
  path: '폴더',
  pathPlaceholder: '~/code/project 또는 절대 경로',
  checking: '확인하는 중…',
  inRepo: 'Git 저장소',
  notRepo: 'Git 저장소가 아니라서 이 폴더에서 바로 작업합니다.',
  notFound: '없는 폴더입니다.',
  notAFolder: '폴더의 절대 경로나 ~로 시작하는 경로를 쓰세요.',
  isolate: 'worktree로 격리',
  isolateHint: '채팅이 자기 브랜치에서 작업하고, 적용하기 전까지 변경이 체크아웃에 들어가지 않습니다.',
  start: '채팅 시작',
  starting: '시작하는 중…',
  cancel: '취소',
  failed: (message) => `채팅을 시작하지 못했습니다: ${message}`,
}
