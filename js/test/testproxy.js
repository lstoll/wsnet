var proxy = require('../src/index.js')("ws://localhost:8080")
proxy.listen(9090, '127.0.0.1');
