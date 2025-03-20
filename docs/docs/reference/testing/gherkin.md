# Gherkin tests

!!!note
    This document is mainly intended for maintainers.

Kaiwo uses Kyverno Chainsaw for end-to-end tests, however testing similar but slightly different scenarios leads to a lot of repetition. To counter this, Kaiwo includes a feature that converts a Gherkin-like YAML test description file into one or more Chainsaw tests.

