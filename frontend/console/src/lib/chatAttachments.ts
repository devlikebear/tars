// Clipboard image extraction and File → ChatAttachment conversion, shared
// by ChatPanel's composer and the focus mode "new task" goal field so a
// pasted screenshot becomes an attachment the same way in both places.
import type { ChatAttachment } from './types'

export const maxChatAttachments = 5

// The slice of DataTransferItem this module needs — narrow so tests can
// build one without a DOM.
export type ClipboardItemLike = {
  type: string
  getAsFile(): File | null
}

// clipboardImageFile turns one clipboard item into a renamed image File,
// or null when the item isn't an image, carries no file, or the caller is
// already at the attachment limit. currentCount is the number of
// attachments already held by the caller (checked before extracting, so a
// paste at the limit never reads the clipboard item for nothing).
export function clipboardImageFile(item: ClipboardItemLike, currentCount: number, max = maxChatAttachments): File | null {
  if (currentCount >= max) return null
  if (!item.type.startsWith('image/')) return null
  const file = item.getAsFile()
  if (!file) return null
  const ext = item.type.split('/')[1] || 'png'
  const name = `clipboard-${Date.now()}.${ext}`
  return new File([file], name, { type: file.type })
}

// clipboardImageFiles extracts every image item from a clipboard paste (a
// screenshot tool can copy more than one), stopping once currentCount plus
// what has been extracted so far reaches max.
export function clipboardImageFiles(items: Iterable<ClipboardItemLike>, currentCount: number, max = maxChatAttachments): File[] {
  const files: File[] = []
  for (const item of items) {
    if (currentCount + files.length >= max) break
    const file = clipboardImageFile(item, 0, 1)
    if (!file) continue
    // Extracting more than one in the same millisecond would otherwise
    // share a name (clipboard-<ts>.<ext>); disambiguate the 2nd+ file.
    const named = files.length === 0 ? file : new File([file], `clipboard-${Date.now()}-${files.length}.${file.name.split('.').pop()}`, { type: file.type })
    files.push(named)
  }
  return files
}

// filesToAttachments base64-encodes files for the chat request body.
export async function filesToAttachments(files: File[]): Promise<ChatAttachment[]> {
  const results: ChatAttachment[] = []
  for (const file of files) {
    const buffer = await file.arrayBuffer()
    const bytes = new Uint8Array(buffer)
    let binary = ''
    for (let i = 0; i < bytes.byteLength; i++) {
      binary += String.fromCharCode(bytes[i])
    }
    results.push({
      name: file.name,
      mime_type: file.type || 'application/octet-stream',
      data: btoa(binary),
    })
  }
  return results
}
