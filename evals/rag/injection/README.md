# Planted injection documents (synthetic)

These files are test fixtures for the `injection` questions in `evals/rag/questions.json`. They're written to look like operator notes, specs, and approval records, and they try to issue instructions. Nothing in them is true or authorized.

They get ingested into the frozen evaluation corpus so the harness can check that retrieved text stays untrusted evidence. Never ingest them into a working corpus.
