function enforceProxy() {
    chrome.enterprise.deviceAttributes.getDeviceHostname(
        function (hostname) {
            if (hostname.startsWith("example-secure")) {
                console.log("enforcing proxy settings...")
                chrome.proxy.settings.set({
                    value: {
                        mode: "pac_script",
                        pacScript: {
                            mandatory: true,
                            url: "https://proxy-config.example.com/authenticated/proxy.pac"
                        }
                    },
                    scope: 'regular'
                }, function () {
                    console.log("done for 'regular'.");
                });
                chrome.proxy.settings.set({
                    value: {
                        mode: "pac_script",
                        pacScript: {
                            mandatory: true,
                            url: "https://proxy-config.example.com/authenticated/proxy.pac"
                        }
                    },
                    scope: 'incognito_persistent'
                }, function () {
                    console.log("done for 'incognito_persistent'.");
                });
            } else {
                console.log("Machine doesn't appear to be a proxy enforced machine, ignoring.")
            }
        }
    )
}

chrome.runtime.onInstalled.addListener(enforceProxy);
chrome.runtime.onStartup.addListener(enforceProxy);