# mem: full project plan that I will implement here

## Ground rules

1. Pure Go with no C dependencies (`CGO_ENABLED=0`), built as one static binary called `mem`. Runs locally, with no background server.
2. Embeddings come from aikit (pure Go, MIT license), running the GTE-Small model (thenlper/gte-small: English only, 384 numbers per vector, MIT license). aikit is a normal module dependency pinned in go.mod, not vendored. The model may be swapped later.
3. Connecting `mem` to specific tools (Claude Code hooks, OpenCode plugins, lines in AGENTS.md) is out of scope. `mem` ships a command line interface and an MCP server. Wiring them up is the user's job.
4. v1 uses no language model at all. `mem` never calls the harness's or a provider's language model. A language model only appears in v2, built into the binary.
5. Tuning is never the user's job. Thresholds are set automatically by `mem` from a built-in example set that ob writes and knows to be correct. Usage decay is the exception: ob sets its values by hand.
6. All memories are written by the agent in the harness (Claude Code or Pi) through `upsert`. `mem` never reads session transcripts. Because of that, the harness must push the agent to save: a Stop hook in Claude Code, and the equivalent in Pi, asks the agent to save decisions and changed facts before it stops. Watch writes per day (from `created_at`): if that number flatlines, the nudge isn't working.
7. Every removal is a hard delete. Nothing is archived, soft-deleted or kept around.
   1. Obsolescence (a memory no longer being needed) is decided only by the usage decay rules in code: how long since a memory was last needed, and how many times it was needed (section 5, item 7). The built-in model never deletes anything for being obsolete.
   2. Other removals: the agent calling `delete`, and merging (v2, item 4), where the merged fact replaces the originals.
8. A daily pass (`mem tidy`) keeps the memories neat and their number manageable. In v1 it's pure code; v2 adds the model steps.
9. Search is fair: ranking uses only similarity and the merge of the two result lists. Usage history never changes how something ranks.
10. Keep it simple.

## v1

### 1. Storage

1. **SQLite** through gorm with the `glebarez/sqlite` driver, which is pure Go (built on `modernc.org/sqlite`). Set these when opening the connection:
   1. **WAL mode** (write-ahead log): readers keep reading while one writer writes. This matters because several sessions use the same file.
   2. **`busy_timeout`:** how long a connection waits for a lock before giving up. 5 seconds is a sane start.
   3. **`foreign_keys`:** SQLite ignores foreign keys unless this is on, and deleting a topic relies on them to delete its facts.
   4. **Immediate write transactions** (`BEGIN IMMEDIATE`): every write transaction takes the write lock at the start. Without it, a transaction that reads first and then writes can fail instantly while another session is writing, and the timeout doesn't help in that case.
2. **Schema changes** are handled by auto migration.
3. **Tables:**
   1. `topics`: id (integer primary key), name (unique), description, embedding, updated_at, last_retrieved_at, hit_count.
   2. `memories`: id (integer primary key), topic_id (required, foreign key to topics, deleted along with its topic), content (at most 280 characters), embedding, created_at, updated_at, last_retrieved_at, hit_count.
   3. `memories_fts`: SQLite's full-text search index over `memories.content`. It reads the text from the memories table instead of keeping its own copy, and three triggers (insert, update, delete) keep it in sync. Use the `porter unicode61` tokenizer: it splits text into words and reduces English words to their stem, so "running" also matches "run".
   4. `injected(session_id, topic_id)`: which topics were already shown in a session, so they aren't shown twice.
   5. `query_log(query, top_scores, injected_ids, ts)`: every query, its scores, and what got shown. For your own debugging.
   6. `settings(key, value)`: what the program needs to run correctly. That's the embedding model name, the vector size, the checksum of the weights, and every number produced by automatic tuning (section 6).
4. **Why integer primary keys:** the search index points at rows by row number. With an integer primary key, that number never changes. Without one, VACUUM can renumber rows and silently break the index.
5. **Embed outside the transaction.** Compute embeddings before starting the transaction, so the write lock isn't held while the model runs. Then, inside one transaction, write the fact and update the topic's embedding.
6. **Backups** with `VACUUM INTO 'backup.db'`, which writes a clean copy of the whole database to a new file.

### 2. Embeddings

1. **aikit, pinned to one version in go.mod.** Why not gte-go (the first choice): its tokenizer only handled ASCII. It never stripped accents and it cut multi-byte characters in half, so text like "café Müller İstanbul" scored 0.81 against the original model and "emoji 🎉 and 中文" scored 0.94. aikit passes both.
2. **Safety copy of the dependency:** the Go module mirror keeps public modules with a recognised license like MIT, but doesn't promise to keep everything forever. Copy aikit's download folder (`$(go env GOMODCACHE)/cache/download/github.com/townsendmerino`) to a backup location. If aikit ever disappears, build with `GOPROXY=file://<backup>,https://proxy.golang.org`.
3. **The model** is the Hugging Face folder as downloaded: `config.json`, `vocab.txt` and `model.safetensors` (the weights are stored as float16). No conversion step.
4. **Accuracy check:** `reference.json` holds vectors from the original Python model (sentence-transformers). Every sentence must score at least 0.999 cosine against aikit's output. The current set passes at 0.99999 or better, including accents, emoji, Chinese, code, an empty string, a single letter and text over the 512 token limit.
   1. Add `Ömer`, `Straße`, `naïve` and `Ağrı`. Names and places will contain these, and `ß` is handled differently from accent marks.
   2. Rerun after every aikit update.
5. **Weights checksum:** keep a SHA-256 checksum of the weights so a corrupted file gets caught.
6. **Model change:** on start, if the model name or weights checksum in `settings` differs from what the binary expects, re-embed every topic and fact, then re-run automatic tuning, before doing anything else. Vectors from two different models can't be compared, so this must never be skipped.
7. **Scale every vector to length 1 in one place,** right where embeddings come back from aikit, so nothing downstream can forget. aikit probably already does it; this is a cheap safety net. Then cosine similarity is just a dot product.
8. **A topic's embedding** is computed from its name and description together.
9. **Startup time:** `mem context` starts a fresh process for every prompt. Measure model loading and one embedding separately. GTE-Small has 12 layers, so expect very roughly 100ms per embedding (aikit reports about 46ms for the 6-layer MiniLM).
   1. If loading is the slow part: hand the prompt over a Unix socket to the `mem mcp` process, which already runs for the whole session with the model loaded. Fall back to loading the model directly when nothing is listening.
   2. If a single embedding is slow by itself, a long-running process won't help. Then the choice is a smaller model or accepting it.

### 3. Search and ranking

1. **Meaning search:** dot product of the query vector against every stored vector. No index needed at this size.
2. **Keyword search:** SQLite's built-in `bm25()`. Raw prompts break the search syntax (hyphens, quotes, colons, and the words AND, OR, NOT, NEAR), so wrap each word in double quotes and join them with OR. The default is AND, which matches almost nothing for a full sentence.
3. **Merge the two result lists by position, not by score** (reciprocal rank fusion). Each fact gets 1 / (60 + its position) from each list, and the two are added. This works even though cosine and bm25 scores are on completely different scales.
4. **A topic's score** is the score of its best fact.
5. **Nothing else affects ranking.** Usage history (last_retrieved_at, hit_count) is only used by the daily pass to decide deletion (section 5, item 7).

### 4. Write rules

1. **Reject facts over 280 characters,** with an error telling the model to shorten.
2. **Choosing the topic for a new fact:**
   1. Exact name match.
   2. Otherwise, compare against each topic's name and description embedding and against its facts, and take the best score.
   3. Unknown name that is close to existing topics: reject with "did you mean X or Y?"
   4. Truly new topic: require a description, then create it.
3. **Duplicate check,** within the topic first, then across all facts:
   1. Above about 0.95 cosine: same fact. Refresh its timestamps and return the existing id.
   2. Between 0.85 and 0.95: possibly the same. Reject and return the similar fact's id so the model decides.
   3. Below 0.85: insert.
   4. These numbers are only fallbacks. GTE gives fairly high scores even to unrelated text, so automatic tuning (section 6) replaces them.
4. **Negation check:** "we use Postgres" and "we don't use Postgres" embed almost identically. If one of two near duplicates contains a negation word (not, no, never, don't and so on) and the other doesn't, don't treat them as duplicates.
5. **Fact cap:** about 30 facts at most per topic. Flag topics that go over it.

### 5. Interfaces

1. **`mem mcp`:** MCP server over stdin and stdout, using the official Go SDK. All logs go to stderr, because anything else printed to stdout corrupts the protocol. Tools:
   1. `upsert(topic, content, id?, topic_description?)`
   2. `search(query, k?, topic?)`
   3. `get(id)`: a topic id returns its description and all its facts. A fact id returns that fact.
   4. `delete(id)`: a fact, or a topic together with all its facts.
   5. `rename(id, name, description)`: for topics.
2. **`mem context --session <id>`:** reads the prompt from stdin and prints what to inject as JSON on stdout. The JSON format carries a version number and the exit codes are fixed, so whatever calls it doesn't break when you change things.
3. **When to inject:** the best match's cosine has to clear a minimum, and it has to be clearly ahead of the second best. Both numbers come from automatic tuning (section 6).
4. **At most 5 entries.** Each is the topic (id, name, description, fact count) plus its best matching fact. Skip topics already in `injected` for this session.
5. **Tool descriptions are instructions for the agent:** write self-contained facts ("ob's PC budget is €2000", not "budget is 2000"), search before writing, and how to answer "did you mean".
6. **`mem export`:** prints every topic and its facts as markdown to stdout, on demand. Read-only, never changes anything. Useful as a readable backup and to diff over time in git.
   1. Format, per topic, with topics sorted by name and facts by `created_at`:

      ```
      # topic name
      ## topic description

      - fact 1
      - fact 2
      ```

   2. A fact containing a newline gets it replaced with a space, so every fact stays one list item.
7. **`mem tidy`:** the daily pass. In v1 it's pure code:
   1. Usage decay: a few if statements with threshold values ob picks by hand. They look at how long since a fact or topic was last retrieved (last_retrieved_at) and how many times it was retrieved (hit_count). Never at how old something is, and never tuned automatically. Facts that decay marks as obsolete are hard-deleted.
   2. Topics left with no facts are deleted.
   3. Topics over the fact cap are listed.
   4. A systemd user timer runs it once a day, with `Persistent=true` so a run missed while the laptop was asleep happens on wake. No daemon.
   5. It holds a file lock, so two passes never run at once.

### 6. Logging and tuning

1. **Log every query** in `query_log` from day one.
2. **Automatic tuning,** never in the user's hands:
   1. The binary ships a built-in example set written by ob, with the correct answer for every example:
      1. Duplicate, near duplicate, related and unrelated fact pairs, plus a few negation pairs. These tune the duplicate thresholds and the consolidation grouping threshold.
      2. Facts with prompts, and whether each prompt should inject them. These tune the injection minimum and gap.
      3. Facts and topics with queries and the results each should return. These tune search, topic resolution and the "did you mean" cutoff.
   2. On first start and after every model change, `mem` runs the set, picks the numbers that get the most examples right, and writes them into `settings`.
   3. Keep the number of tuned values small compared to the number of examples, and check the result on a part of the set that tuning didn't see. Otherwise the numbers just memorize the examples and fail on real data.
3. **Test queries,** for you during development: about 30 queries with their expected results. Run them after every ranking change so you notice when something gets worse.

## v2: continuous refinement

The goal: the daily pass keeps going over the memories and refines them. It merges duplicates, splits topics that grew too big, merges topics that drifted together, and finds groups held together by purpose rather than subject ("things to tell Dirk"). Embeddings can't see purpose, so this needs a language model. Code does everything it can first, and the model reviews and fixes what code proposes. Reviewing a proposal is far easier for a small model than building groups from scratch. The model never deletes anything for being obsolete (ground rule 7).

1. **Built-in language model:** goinfer (a pure Go library that runs language models) with Gemma 4 E2B (Google's small model, Apache 2.0 license, about 1.5GB). Move to E4B (bigger, needs a few GB of RAM) only if E2B fails on the test cases.
   1. Download the weights on first run with a SHA-256 check instead of embedding them in the binary.
   2. Pin goinfer to a fixed version. It's a small project, not yet at version 1.0.
   3. Put it behind a small Go interface of your own so it can be swapped.
   4. Temperature 0 (the model always picks its most likely next word, so the same input gives the same output) and constrained output (the model can only produce text that matches a fixed JSON shape). Validate the result anyway.
2. **Never on a path anyone waits for.** On CPU in pure Go it will probably manage only a few words per second, so `mem context` and `mem mcp` never load it. It only runs inside `mem tidy`, which holds a file lock so two passes never run at once.
3. **The model steps run inside the daily pass** (`mem tidy`), after the v1 decay deletions.
   1. Add `last_consolidated_at` to topics. The model steps only look at topics changed since their last pass, so a daily run stays quick.
4. **Merging duplicate facts within a topic:**
   1. Code groups near duplicates: if A is close to B and B is close to C, all three land in one group (union find). If there are no groups, skip the model entirely.
   2. If the facts in a group say the same thing, keep the newest (it's the current truth) and hard-delete the rest. Pure code.
   3. If the facts in a group carry different information, the model combines them into one new fact, which gets embedded, and the originals are hard-deleted.
5. **Splitting a topic that went over the fact cap:**
   1. Code proposes the split: divide the facts into two groups by cosine similarity, closest together in the same group.
   2. The model reviews it: confirms the split, moves facts that landed on the wrong side, and writes a name and description for each new topic.
6. **Merging topics:** when two topics' embeddings have become nearly identical, the model confirms before their facts are moved into one topic. Code then deletes the emptied topic.
7. **Logical grouping:** the model reviews facts across topics for groups held together by purpose, not subject, and proposes moving them into an existing or new topic. Every fact still belongs to exactly one topic, so a moved fact leaves its old topic. This is the hardest job for a small model, so test E2B on it specifically.
8. **Descriptions:** after any change, the model rewrites the descriptions of the affected topics, and their embeddings are recomputed.
9. **Change log, for debugging:** every change a pass makes (combined facts, deleted duplicates, moves, splits, topic merges, rewritten descriptions) is logged with what it looked like before and after.
10. **Tuning,** the same way as section 6 of v1: built-in examples with known answers for merge, split and grouping decisions, run automatically.

## Maybe later

1. Letting users adjust the usage decay thresholds.
2. Letting a fact belong to more than one topic (tags), if moving facts out of their subject topic during logical grouping turns out to lose too much.

## Not planned

1. Reading session transcripts (`mem extract`). The agent writing through `upsert` has the full context and is a far better model.
2. Approximate nearest neighbour indexes, SQLite vector extensions, multilingual support, a separate background server, cloud.
