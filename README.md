# mem: full project plan that I will implement here

## Ground rules

1. Pure Go with no C dependencies (`CGO_ENABLED=0`), built as one static binary called `mem`. Runs locally, with no background server.
2. Embeddings come from gte-go, a pure Go port of the GTE-Small model. English only, 384 numbers per vector, MIT license, copied into the repo. The model may be swapped later.
3. Connecting `mem` to specific tools (Claude Code hooks, OpenCode plugins, lines in AGENTS.md) is out of scope. `mem` ships a command line interface and an MCP server. Wiring them up is the user's job.
4. v1 uses no language model at all. `mem` never calls the harness's or a provider's language model. A language model only appears in v2, built into the binary.
5. Tuning is never the user's job. `mem` sets thresholds automatically from a built-in example set that you write and know to be correct. Usage decay is the exception: you set its values by hand.
6. Keep it simple.

## v1

### 1. Storage

1. Use `modernc.org/sqlite` (SQLite in pure Go). Set these in the connection string:
   1. WAL mode (write ahead log): readers keep reading while one writer writes. This matters because several sessions use the same file.
   2. `busy_timeout`: how long a connection waits for a lock before giving up. 5 seconds is a sane start.
   3. `foreign_keys`: SQLite ignores foreign keys unless this is on, and deleting a topic relies on them to delete its facts.
   4. `_txlock=immediate`: every write transaction takes the write lock at the start. Without it, a transaction that reads first and then writes can fail instantly while another session is writing, and the timeout doesn't help in that case.
2. Schema changes are handled by auto migration.
3. Tables:
   1. `topics`: id (integer primary key), name (unique), description, embedding, updated_at, last_retrieved_at, hit_count.
   2. `memories`: id (integer primary key), topic_id (required, foreign key to topics, deleted along with its topic), content (at most 280 characters), embedding, created_at, updated_at, last_retrieved_at, hit_count.
   3. `memories_fts`: SQLite's full text search index over `memories.content`. It reads the text from the memories table instead of keeping its own copy, and three triggers (insert, update, delete) keep it in sync. Use the `porter unicode61` tokenizer: it splits text into words and reduces English words to their stem, so "running" also matches "run".
   4. `injected(session_id, topic_id)`: which topics were already shown in a session, so they aren't shown twice.
   5. `query_log(query, top_scores, injected_ids, ts)`: every query, its scores, and what got shown. For your own debugging.
   6. `settings(key, value)`: what the program needs to run correctly. That's the embedding model name, the vector size, the checksum of the weights, and every number produced by automatic tuning (section 6).
4. **Why integer primary keys:** the search index points at rows by row number. With an integer primary key that number never changes. Without one, VACUUM can renumber rows and silently break the index.
5. Compute embeddings before starting the transaction, so the write lock isn't held while the model runs. Then, inside one transaction, write the fact and update the topic's embedding.
6. Back up with `VACUUM INTO 'backup.db'`, which writes a clean copy of the whole database to a new file.

### 2. Embeddings

1. Copy gte-go into the repo and keep its LICENSE file.
2. **Accuracy check:** embed about 20 sentences with the original Python model (sentence-transformers) and with gte-go. The cosine between each matching pair should be above 0.999. If it isn't, the port is wrong somewhere.
3. Ship the model weights with a SHA-256 checksum so a corrupted file gets caught.
4. **Model change:** on start, compare the model name and weights checksum in `settings` with what the binary ships. If they differ, re-embed every topic and fact, then re-run automatic tuning, before doing anything else. Vectors from two different models can't be compared, so this must never be skipped. At a few thousand facts and about 10ms each, this takes tens of seconds, once per model change.
5. Scale every vector to length 1 before storing it. Then cosine similarity is just a dot product.
6. A topic's embedding is computed from its name and description together.
7. **Measure startup time.** `mem context` starts a fresh process for every prompt, so the model weights load every time. gte-go's 10ms per embedding most likely doesn't include that.

### 3. Search and ranking

1. **Meaning search:** dot product of the query vector against every stored vector. No index needed at this size.
2. **Keyword search:** SQLite's built-in `bm25()`. Raw prompts break the search syntax (hyphens, quotes, colons, and the words AND, OR, NOT, NEAR), so wrap each word in double quotes and join them with OR. The default is AND, which matches almost nothing for a full sentence.
3. **Merging the two lists:** merge by position, not by score (reciprocal rank fusion). Each fact gets 1 / (60 + its position) from each list, and the two are added. This works even though cosine and bm25 scores are on completely different scales.
4. **Usage decay:** a few if statements with threshold values you pick by hand, based on last_retrieved_at and hit_count, applied to both facts and topics. Facts and topics that haven't been retrieved in a while, or are rarely retrieved, rank a bit lower. Never based on how old something is. Not tuned automatically.
5. A topic's score is the score of its best fact.

### 4. Write rules

1. Reject facts over 280 characters, with an error telling the model to shorten.
2. **Choosing the topic for a new fact:**
   1. Try an exact name match.
   2. Otherwise, compare against each topic's name and description embedding and against its facts, and take the best score.
   3. If the name is unknown but close to existing topics, reject with "did you mean X or Y?"
   4. For a truly new topic, require a description, then create it.
3. **Duplicate check,** within the topic first, then across all facts:
   1. Above about 0.95 cosine: same fact. Refresh its timestamps and return the existing id.
   2. Between 0.85 and 0.95: possibly the same. Reject and return the similar fact's id so the model decides.
   3. Below 0.85: insert.
   4. These numbers are only fallbacks. GTE gives fairly high scores even to unrelated text, so automatic tuning (section 6) replaces them.
4. **Negation check:** "we use Postgres" and "we don't use Postgres" embed almost identically. If one of two near duplicates contains a negation word (not, no, never, don't and so on) and the other doesn't, don't treat them as duplicates.
5. About 30 facts at most per topic. Flag topics that go over it.

### 5. Interfaces

1. `mem mcp`: an MCP server over stdin and stdout, using the official Go SDK. All logs go to stderr, because anything else printed to stdout corrupts the protocol. Tools:
   1. `upsert(topic, content, id?, topic_description?)`
   2. `search(query, k?, topic?)`
   3. `get(id)`: a topic id returns its description and all its facts. A fact id returns that fact.
   4. `delete(id)`: a fact, or a topic together with all its facts.
   5. `rename(id, name, description)`: for topics.
2. `mem context --session <id>`: reads the prompt from stdin and prints what to inject as JSON on stdout. The JSON format carries a version number and the exit codes are fixed, so whatever calls it doesn't break when you change things.
3. **When to inject:** the best match's cosine has to clear a minimum, and it has to be clearly ahead of the second best. Both numbers come from automatic tuning (section 6).
4. At most 5 entries. Each is the topic (id, name, description, fact count) plus its best matching fact. Skip topics already in `injected` for this session.
5. **Tool descriptions are instructions for the agent:** write self-contained facts ("ob's PC budget is €2000", not "budget is 2000"), search before writing, and how to answer "did you mean".

### 6. Logging and tuning

1. Log every query in `query_log` from day one.
2. **Automatic tuning, never in the user's hands:**
   1. The binary ships a built-in example set you write, with the correct answer for every example:
      1. Duplicate, near duplicate, related and unrelated fact pairs, plus a few negation pairs. These tune the duplicate thresholds and the consolidation grouping threshold.
      2. Facts with prompts, and whether each prompt should inject them. These tune the injection minimum and gap.
      3. Facts and topics with queries and the results each should return. These tune search, topic resolution and the "did you mean" cutoff.
   2. On first start and after every model change, `mem` runs the set, picks the numbers that get the most examples right, and writes them into `settings`.
   3. Keep the number of tuned values small compared to the number of examples, and check the result on a part of the set that tuning didn't see. Otherwise the numbers just memorize the examples and fail on real data.
3. **Test queries,** for you during development: about 30 queries with their expected results. Run them after every ranking change so you notice when something gets worse.

## v2

1. **Built-in language model:** goinfer (a pure Go library that runs language models) with Gemma 4 E2B (Google's small model, Apache 2.0 license, about 1.5GB). Move to E4B (bigger, needs a few GB of RAM) only if E2B fails on your test cases.
   1. Download the weights on first run with a SHA-256 check instead of embedding them in the binary.
   2. Copy goinfer into the repo at a fixed commit. It's a small project, not yet at version 1.0.
   3. Put it behind a small Go interface of your own so it can be swapped.
   4. Use temperature 0 (the model always picks its most likely next word, so the same input gives the same output) and constrained output (the model can only produce text that matches a fixed JSON shape). Validate the result anyway.
2. **Never on a path anyone waits for.** On CPU in pure Go it will probably manage only a few words per second, so `mem context` never loads it. Run one background job at a time, using a file lock or a jobs table that a single worker processes. Otherwise several sessions ending together would each load gigabytes of weights.
3. `mem extract`: reads a conversation transcript from stdin. What triggers it is the harness's business.
   1. Output is strict JSON: at most 5 facts, each with a score for how long it will stay true. Drop low scores.
   2. **Topic:** code finds the 3 nearest topics, and the model picks one of them or "new" with a description.
   3. Every fact goes through the same write rules as a manual upsert.
4. **Unclear topic choice, background jobs only:** when the two best topics score too close, give the model the 3 nearest and let it pick. A faster variant: in one pass, read the model's confidence for each answer letter instead of letting it write text. That also gives you a number to set a minimum on. A manual upsert keeps returning "did you mean" to the agent.
5. `mem consolidate`:
   1. Group near duplicate facts within each topic in pure code: if A is close to B and B is close to C, all three land in one group (union find). If there are no groups, skip the model entirely.
   2. The model merges each group into one fact, which then gets re-embedded.
   3. Merge topics whose embeddings have become nearly identical. Split topics over the fact cap.
   4. Rewrite the descriptions and embeddings of changed topics.
6. Tune the minimum durability score the same way as section 6 of v1: built-in examples with known answers, run automatically.

## Maybe later

1. Letting users adjust the usage decay thresholds.

## Not planned

Approximate nearest neighbour indexes, SQLite vector extensions, multilingual support, a background server, cloud.
