#!/bin/bash

openssl genrsa -out ca.key 2048

openssl req -new -x509 -days 365 -key ca.key \
  -subj "/CN=namespace-termination-locker"\
  -out ca.crt

openssl req -newkey rsa:2048 -nodes -keyout server.key \
  -subj "/CN=namespace-termination-locker-webhook" \
  -out server.csr

echo
echo ">> SAN is 'namespace-termination-locker-webhook.default.svc'"
openssl x509 -req \
  -extfile <(printf "subjectAltName=DNS:namespace-termination-locker-webhook.default.svc") \
  -days 365 \
  -in server.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out server.crt

echo
echo ">> Generating kube secrets..."
kubectl create secret tls namespace-termination-locker-tls \
  --cert=server.crt \
  --key=server.key \
  --dry-run=client -o yaml \
  > manifests/Secret.yaml

echo
echo ">> Please update caBundle of the validatingwebhookconfiguration manually:"
cat ca.crt | base64 

rm ca.crt ca.key ca.srl server.crt server.csr server.key