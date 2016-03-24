// reades packets from stdin to test the writer implementation
var pull = require('pull-stream')
var psc = require('packet-stream-codec')
var toPull = require('stream-to-pull-stream')

pull(
  toPull.source(process.stdin),
  psc.decode(),
  pull.collect(function (err, array) {
    if (err) throw err
    console.log(JSON.stringify(array))
  })
)
