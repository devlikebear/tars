import test from 'node:test'
import assert from 'node:assert/strict'

import { clipboardImageFile, clipboardImageFiles, filesToAttachments, maxChatAttachments, type ClipboardItemLike } from '../src/lib/chatAttachments.ts'

function imageItem(mime = 'image/png', bytes = 'pixel'): ClipboardItemLike {
  return {
    type: mime,
    getAsFile: () => new File([bytes], 'ignored.png', { type: mime }),
  }
}

function fileItem(mime: string): ClipboardItemLike {
  return { type: mime, getAsFile: () => new File(['x'], 'ignored.txt', { type: mime }) }
}

function nullFileItem(mime = 'image/png'): ClipboardItemLike {
  return { type: mime, getAsFile: () => null }
}

test('clipboardImageFile renames an image item clipboard-<ts>.<ext>', () => {
  const file = clipboardImageFile(imageItem('image/png'), 0)
  assert.ok(file)
  assert.match(file.name, /^clipboard-\d+\.png$/)
  assert.equal(file.type, 'image/png')
})

test('clipboardImageFile derives the extension from the mime subtype', () => {
  const file = clipboardImageFile(imageItem('image/jpeg'), 0)
  assert.ok(file)
  assert.match(file.name, /\.jpeg$/)
})

test('clipboardImageFile ignores a non-image item', () => {
  assert.equal(clipboardImageFile(fileItem('text/plain'), 0), null)
})

test('clipboardImageFile returns null when there is no file on the item', () => {
  assert.equal(clipboardImageFile(nullFileItem(), 0), null)
})

test('clipboardImageFile enforces the attachment limit', () => {
  assert.equal(clipboardImageFile(imageItem(), maxChatAttachments), null)
  assert.ok(clipboardImageFile(imageItem(), maxChatAttachments - 1))
  assert.ok(clipboardImageFile(imageItem(), 2, 10), 'a smaller custom max is not yet reached')
})

test('clipboardImageFiles extracts every image item up to the limit', () => {
  const items = [imageItem('image/png'), fileItem('text/plain'), imageItem('image/jpeg')]
  const files = clipboardImageFiles(items, 0)
  assert.equal(files.length, 2)
  assert.match(files[0].name, /\.png$/)
  assert.match(files[1].name, /\.jpeg$/)
})

test('clipboardImageFiles stops once currentCount reaches max', () => {
  const items = [imageItem(), imageItem(), imageItem()]
  const files = clipboardImageFiles(items, maxChatAttachments - 1)
  assert.equal(files.length, 1)
})

test('clipboardImageFiles gives distinct names when extracting more than one', () => {
  const files = clipboardImageFiles([imageItem('image/png'), imageItem('image/png')], 0)
  assert.equal(files.length, 2)
  assert.notEqual(files[0].name, files[1].name)
})

test('filesToAttachments base64-encodes file content with name and mime type', async () => {
  const file = new File(['hello'], 'greeting.txt', { type: 'text/plain' })
  const [attachment] = await filesToAttachments([file])
  assert.equal(attachment.name, 'greeting.txt')
  assert.equal(attachment.mime_type, 'text/plain')
  assert.equal(Buffer.from(attachment.data, 'base64').toString('utf8'), 'hello')
})

test('filesToAttachments falls back to application/octet-stream when the file has no type', async () => {
  const file = new File(['x'], 'blob')
  const [attachment] = await filesToAttachments([file])
  assert.equal(attachment.mime_type, 'application/octet-stream')
})

test('filesToAttachments preserves order across multiple files', async () => {
  const files = [new File(['a'], 'a.txt', { type: 'text/plain' }), new File(['b'], 'b.txt', { type: 'text/plain' })]
  const attachments = await filesToAttachments(files)
  assert.deepEqual(attachments.map((a) => a.name), ['a.txt', 'b.txt'])
})
