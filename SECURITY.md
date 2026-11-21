# Security policy

BreeferCP runs with root privileges on internet-facing servers, so please report vulnerabilities privately and give us a chance to fix them before disclosure.

**Do not open a public issue for a security problem.** Use GitHub's *Report a vulnerability* (private advisory) on this repository. Include the affected version, reproduction steps and impact.

You can expect an acknowledgement within 3 working days and a fix or mitigation plan within 30 days for confirmed issues. Reporters are credited in the advisory unless they prefer otherwise.

In scope: authentication and session handling, authorisation and tenant isolation, command or config injection in the provisioner, the file manager jail, the billing API and SSO. Out of scope: issues that need an already-compromised root account, and findings in third-party software we merely configure (nginx, Postfix, and so on) unless our configuration makes them exploitable.

See [docs/security.md](docs/security.md) for the current security model and its known gaps.
