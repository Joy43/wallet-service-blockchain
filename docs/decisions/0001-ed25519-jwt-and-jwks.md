# 0001: Sign access tokens with Ed25519 and publish a JWKS

## Context
Access tokens were HS256: every service that verified a token needed the same shared secret,
so compromising any one service allowed forging tokens for all of them.

## Decision
auth-service signs with an Ed25519 private key (EdDSA). It publishes the public key at
`GET /.well-known/jwks.json`. Other services verify tokens locally with that key.
The signing key exists only in auth-service. Tokens carry a `kid`, so keys can be rotated.

## Consequences
- Other services need no secret and no call to auth for ordinary requests.
- A locally verified token stays valid until it expires (15 minutes) even after logout.
  Services that need immediate revocation (money movement) should still call gRPC `ValidateToken`,
  which also checks that the session exists.
- Key rotation is not automated yet: add the new key to the JWKS before signing with it.
