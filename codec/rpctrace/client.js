/*
This file is part of go-muxrpc.

go-muxrpc is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

go-muxrpc is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with go-muxrpc.  If not, see <http://www.gnu.org/licenses/>.
*/

var MRPC = require('muxrpc')
var pull = require('pull-stream')
var toPull = require('stream-to-pull-stream')

var api = {
  hello: 'async',
  stuff: 'source'
}

var client = MRPC(api, null)()
var a = client.createStream()

pull(a, toPull.sink(process.stdout))
pull(toPull.source(process.stdin), a)

var cnt = 0
var helloInt = setInterval(function () {
  client.hello('world' + cnt, function (err, value) {
    if (err) {
      console.error('hello:failed')
      return
    }
    console.error(value) // hello, world!
  })
  cnt++
}, 1000)

setInterval(function () {
  pull(client.stuff(), pull.drain(console.error))
}, 2000)
