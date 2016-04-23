var net = require('net');
var WebSocket = require('ws');

module.exports = function(upstream) {
  var server = net.createServer(function(socket) {
    var ws = new WebSocket(upstream, {
      origin: 'http://localhost'
    });

    var dataFrame = 0x00;
    var heartbeatFrame = 0x01;

    var buffer = [];

    socket.on('data', function(data) {
      console.log(data.constructor.name);
      if (!ws.readyState == WebSocket.OPEN) {
	buffer.push(data);
      } else {
	console.log("gonna send")
	ws.send(Buffer.concat([new Buffer([dataFrame]), data]), {binary: true});
      }
    });

    socket.on('close', function() {
      if (ws.readyState != WebSocket.CLOSED) {
	ws.close();
      }
    });

    ws.on('open', function() {
      console.log("ws opened");
      if (buffer.length) {
	ws.send(dataFrame + buffer, {binary: true});
	buffer = [];
      }
    });

    ws.on('message', function(data, flags) {
      // flags.binary will be set if a binary data is received.
      if (data[0] == dataFrame) {
	// Send down TCP
	socket.write(data.slice(1));
      } else if (data[0] == heartbeatFrame) {
	// NOOP
      } else {
	throw "what is this packet";
      }
    });
  });

  return server;
}
