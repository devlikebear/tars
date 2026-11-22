// The page fake-preview.sh serves: what the result screen shows in place of
// the person's app.
import { createServer } from 'node:http'

const port = Number(process.argv[2])
createServer((_req, res) => {
  res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
  res.end('<!doctype html><title>Book club</title><h1>Book club sign-up</h1>')
}).listen(port, '127.0.0.1')
