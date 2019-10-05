// SPDX-License-Identifier: MIT



var MRPC = require('muxrpc')
var pull = require('pull-stream')
var toPull = require('stream-to-pull-stream')
var split = require('pull-randomly-split')

var api = {
  hello: 'async',
 stuff: 'source'
}

var i = 0
function upTo (i) {
  var arr = []
  for (var n = 0; n < i; n++) {
    arr.push(n)
  }
  return arr
}

var server = MRPC(null, api)({
  hello: function (name, cb) {
    if (Math.random() > 0.5) {
        console.error('hello:ok')
        cb(null, 'hello, ' + name + '!')
    } else {
        console.error('hello:err')
        cb(Error("hello:failed:"+name),null)
    }
  },
  stuff: function () {
    console.error('stuff called:' + i)
    arr = upTo(i)
    i++
    return pull.values(arr)
  }
})

var a = server.createStream()
pull(a, split(), toPull.sink(process.stdout))
pull(toPull.source(process.stdin), a)
