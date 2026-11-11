# ADR-0007: Integrate a mail server instead of building one

Status: Proposed

## Context
Email hosting is a feature of CyberPanel. Deliverability, spam handling and abuse management are a large ongoing burden.

## Decision
Do not implement SMTP or IMAP. The mail module manages domains, mailboxes and DNS records for an integrated server (Stalwart or equivalent).

## Consequences
Fast to deliver and lower risk. Depends on the chosen server's API and licence. Targeted for v1.0.
