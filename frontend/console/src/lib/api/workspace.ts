import { requestJSON } from './client.ts'

// --- Workspace Files ---

export type WorkspaceFileEntry = {
  name: string
  path: string
  is_dir: boolean
  size?: number
  updated_at?: string
}

export type WorkspaceFileContent = {
  path: string
  name: string
  size: number
  updated_at: string
  kind: 'text' | 'markdown' | 'image' | 'binary'
  mime_type: string
  encoding?: 'utf-8' | 'base64'
  content?: string
  content_base64?: string
  truncated?: boolean
  is_binary?: boolean
  message?: string
}

function workspaceFilesEndpoint(root?: string): string {
  return root ? '/v1/filesystem/files' : '/v1/workspace/files'
}

export async function listWorkspaceFiles(path = '.', root?: string): Promise<{ path: string; files: WorkspaceFileEntry[] }> {
  const params = new URLSearchParams({ path })
  if (root) params.set('root', root)
  return requestJSON<{ path: string; files: WorkspaceFileEntry[] }>(
    `${workspaceFilesEndpoint(root)}?${params}`
  )
}

export async function readWorkspaceFile(path: string, root?: string): Promise<WorkspaceFileContent> {
  const params = new URLSearchParams({ path })
  if (root) params.set('root', root)
  return requestJSON<WorkspaceFileContent>(
    `${workspaceFilesEndpoint(root)}?${params}`
  )
}

export async function createWorkspaceDirectory(parentPath: string, name: string, root?: string): Promise<{ path: string; name: string; is_dir: boolean }> {
  const params = new URLSearchParams()
  if (root) params.set('root', root)
  return requestJSON<{ path: string; name: string; is_dir: boolean }>(
    `${workspaceFilesEndpoint(root)}?${params}`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ parent_path: parentPath, name }),
    },
  )
}

export async function renameWorkspaceDirectory(path: string, newName: string, root?: string): Promise<{ path: string; name: string; is_dir: boolean }> {
  const params = new URLSearchParams()
  if (root) params.set('root', root)
  return requestJSON<{ path: string; name: string; is_dir: boolean }>(
    `${workspaceFilesEndpoint(root)}?${params}`,
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path, new_name: newName }),
    },
  )
}

// --- Filesystem ---

export type FilesystemBrowseResult = {
  path: string
  parent: string
  entries: { name: string; is_dir: boolean; is_git?: boolean }[]
}

export async function browseFilesystem(path?: string): Promise<FilesystemBrowseResult> {
  const params = path ? `?path=${encodeURIComponent(path)}` : ''
  return requestJSON(`/v1/filesystem/browse${params}`)
}

export async function createFilesystemDirectory(parentPath: string, name: string): Promise<{ path: string; name: string; is_dir: boolean }> {
  return requestJSON('/v1/filesystem/browse', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ parent_path: parentPath, name }),
  })
}
