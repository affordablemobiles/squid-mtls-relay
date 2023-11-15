# Squid mTLS Relay

A service that converts a conventional Squid forward proxy into one that is both accessible over HTTPS (TLSv1.3) and supports mTLS (Mutual TLS aka Client Certificate) authentication.

Written in Go, as a memory safe language with a trusted TLS stack, it is hopefully more reliably secure when opening the endpoint up to the internet on an external IP, as designed.

HTTP/2 from client -> relay is also supported out of the box.

## Setup

Build and run with Docker.

At runtime, make sure these files are mounted and accessible:

* `cacert.pem` (CA certificate(s) for mTLS)
* `cert.pem` (SSL/TLS Server Certificate)
* `key.pem` (SSL/TLS Server Certificate Key)

Configure the `PROXY_ADDR` environment variable as `host:port`, e.g. `127.0.0.1:3128` when running both the relay & squid within a pod on Kubernetes.

Also, configure the `CERTIFICATE_DNS_SUFFIX` environment variable:

User / Device identify is pulled from the certificate SAN DNSNames field, and the `CERTIFICATE_DNS_SUFFIX` is removed from the value, i.e.:

If your device with serial `S234K2A` is issued a certificate as `S234K2A.example.com`, set this value to `.example.com`.

Optionally, configure the environment variable `PORT` if you'd like to listen on something other than `8443`.

## Squid Configuration

See the [examples/docker-squid](examples/docker-squid) folder for an example of Dockerized Squid, with a partial configuration that shows how to configure the ACLs and authentication for use with this relay.

## Browser configuration

This has mainly been tested against Chrome on Chrome OS.

You'll need to use a auto-configuration script to specify a `HTTPS` proxy, see our example in [examples/proxy.pac](examples/proxy.pac).

It is helpful to configure the [AutoSelectCertificateForUrls](https://chromeenterprise.google/policies/?policy=AutoSelectCertificateForUrls) policy so that users aren't prompted to select the client certificate to use when connecting to the proxy, as you can hint which one should be selected automatically.

If you'd like to apply the proxy configuration per device, rather than per user, it is useful to use a Chrome Extension deployed via Enterprise policy: our [example](examples/chrome-extension) checks the prefix of the hostname configured with the [DeviceHostnameTemplate](https://chromeenterprise.google/intl/en_uk/policies/#DeviceHostnameTemplate) policy.

## Usage with another proxy

There is no reason this relay won't work with a forward proxy other than Squid, but this hasn't been tested.

You would need to:

* Accept HTTP Basic authentication with:
   * Certificate Identity (described above with CERTIFICATE_DNS_SUFFIX) as the username
   * `automatic` is always set as the password
* Take the user IP from the X-Real-IP header for your logs.