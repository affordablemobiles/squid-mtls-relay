function FindProxyForURL(url, host) {
    var direct = 'DIRECT';
    var unauthenticated = 'PROXY 172.16.2.1:3128';
    var proxy = 'HTTPS proxy.example.com:443';

    var proxyList = {
        'example.local': 1
    };

    var proxySpecificList = {
        'dl.google.com ': 1,
        'dl-ssl.google.com': 1
    };

    if (isPlainHostName(host)||
        host === '127.0.0.1'||
        host === 'localhost') {
        return direct;
    }

    if (proxySpecificList.hasOwnProperty(host)) {
        return unauthenticated;
    }

    var pos = host.lastIndexOf('.') + 1;

    do {
        hostStr = host.substring(pos);

        if (proxyList.hasOwnProperty(hostStr)) {
            return direct;
        }

        pos = host.lastIndexOf('.', pos - 2) + 1;
    } while (hostStr != host);

    return proxy;
}