# mem: full project plan that I will implement here

## Ground rules

1. Pure Go with no C dependencies (`CGO_ENABLED=0`), built as one static binary called `mem`. Runs locally, with no background server.
2. Embeddings come from aikit (pure Go, MIT license), running the GTE-Small model (thenlper/gte-small: English only, 384 numbers per vector, MIT license). aikit is a normal module dependency pinned in go.mod, not vendored. The model and the aikit version stay as they are unless one turns out to be bad.
3. Connecting `mem` to specific tools (Claude Code hooks, OpenCode plugins, lines in AGENTS.md) is out of scope. `mem` ships a command line interface and an MCP server. Wiring them up is the user's job.
4. v1 uses no language model at all. `mem` never calls the harness's or a provider's language model. A language model only appears in v2, built into the binary.
5. Tuning comes from a tool that is run by hand. It picks the thresholds from an example set that ob writes and knows to be correct, and writes them to a JSON file that's compiled into the binary (section 6). Usage decay is the exception: ob sets its values by hand.
6. All memories are written by the agent in the harness (Claude Code or Pi) through their respective create tools. `mem` never reads session transcripts. Because of that, the harness must push the agent to save: a Stop hook in Claude Code, and the equivalent in Pi, asks the agent to save decisions and changed facts before it stops. Watch writes per day (from `created_at`): if that number flatlines, the nudge isn't working.
7. Every removal is a hard delete. Nothing is archived, soft-deleted or kept around.
   1. Obsolescence (a memory no longer being needed) is decided only by the usage decay rules in code: how long since a memory was last needed, and how many times it was needed (section 5, item 6). The built-in model never deletes anything for being obsolete.
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
   4. `injections(session_id, memory_id, created_at)`: which memories were already shown in a session, so they aren't shown twice. `created_at` lets old sessions' rows be cleaned up later.
   5. `events(id, kind, payload, ts)`: one row per search, injection and write tool call, for your own debugging and for calibration. `kind` says which it was, and `payload` is JSON whose shape depends on `kind`. It's stored as JSON text, not SQLite's binary JSONB, so it stays readable in any SQLite viewer.
      1. `search`: the query, plus the returned memory ids with their cosine and fusion scores.
      2. `inject`: the prompt, the session id, the top scores, and the memory ids that got shown.
      3. Write tools (`create_topic`, `update_topic`, `create_memory`, `update_memory`): the tool input, the outcome (written, rejected, error), the rejection or error text, and the fit score once it exists.
4. **Why integer primary keys:** the search index points at rows by row number. With an integer primary key, that number never changes. Without one, VACUUM can renumber rows and silently break the index.
5. **Embed outside the transaction.** Compute embeddings before starting the transaction, so the write lock isn't held while the model runs. Then, inside one transaction, write the fact and update the topic's embedding.
6. **Backups** with `VACUUM INTO 'backup.db'`, which writes a clean copy of the whole database to a new file.

### 2. Embeddings

1. **aikit, pinned to one version in go.mod.** Why not gte-go (the first choice): its tokenizer only handled ASCII. It never stripped accents and it cut multi-byte characters in half, so text like "café Müller İstanbul" scored 0.81 against the original model and "emoji 🎉 and 中文" scored 0.94. aikit passes both.
2. **Safety copy of the dependency:** the Go module mirror keeps public modules with a recognised license like MIT, but doesn't promise to keep everything forever. Keep a git clone of the exact aikit tag pinned in go.mod outside the repo, and re-clone it after every aikit update. If aikit ever disappears, add `replace github.com/townsendmerino/aikit => <path to the clone>` to go.mod. A local replace isn't checked against go.sum, so the clone has to stay on the pinned tag.
3. **The model** is the Hugging Face folder as downloaded: `config.json`, `vocab.txt`, `tokenizer.json` and `model.safetensors` (the weights are stored as float16). No conversion step.
   1. The files are compiled into the binary (`go:embed`), so `mem` runs anywhere without a model folder. This adds about 67MB to the binary.
   2. aikit's `LoadBERT` only loads from a folder, so on start the files are written to a temporary folder and loaded from there.
   3. `LoadBERT` refuses to start without `tokenizer.json`.
4. **Accuracy check:** `reference.json` holds vectors from the original Python model (sentence-transformers). Every sentence must score at least 0.999 cosine against aikit's output. The current set passes at 0.99999 or better, including accents, emoji, Chinese, code, an empty string, a single letter and text over the 512 token limit.
   1. Add `Ömer`, `Straße`, `naïve` and `Ağrı`. Names and places will contain these, and `ß` is handled differently from accent marks.
   2. Rerun after every aikit update.
5. **Scale every vector to length 1 in one place,** right where embeddings come back from aikit, so nothing downstream can forget. aikit probably already does it; this is a cheap safety net. Then cosine similarity is just a dot product.
6. **A topic's embedding** is computed from its name.

### 3. Search and ranking

1. **Meaning search:** dot product of the query vector against every stored vector. No index needed at this size.
2. **Keyword search:** SQLite's built-in `bm25()`. Raw prompts break the search syntax (hyphens, quotes, colons, and the words AND, OR, NOT, NEAR), so wrap each word in double quotes and join them with OR. The default is AND, which matches almost nothing for a full sentence.
3. **Merge the two result lists by position, not by score** (reciprocal rank fusion). Each fact gets 1 / (60 + its position) from each list, and the two are added. This works even though cosine and bm25 scores are on completely different scales.
4. **A topic's score** is the score of its best fact.
5. **Nothing else affects ranking.** Usage history (last_retrieved_at, hit_count) is only used by the daily pass to decide deletion (section 5, item 6).

### 4. Write rules

Four tools, two shared checks.

#### Shared checks

1. **Memory check** (skipped when `force` is set).
   1. Search all memories by raw cosine against the content embedding, top k, no threshold. Exclude the memory itself when updating.
   2. Every memory at or above the memory threshold (0.85 fallback): reject and return them, each with memory id, content, cosine score, updated_at, topic id, topic name.
   3. Error message tells the model its options: update one of them with `update_memory`, or resend with `force` if this is a different fact.
   4. Keep the full top k result for the fit score below.

2. **Topic name check** (skipped when `force` is set).
   1. Exact name match on another topic: reject with "topic X already exists" and its id. `force` does not skip this one.
   2. Otherwise compare the name embedding against every other topic's name embedding.
   3. Topics at or above the topic threshold: reject with "did you mean X or Y?", returning topic id, name, description, memory count. Tell the model to use one of those, or resend with `force`.

3. **Fit score** (skipped when `force` is set).
   1. Use the memory check's top k, excluding the memory itself when updating.
   2. Compare the best match overall against the chosen topic's best memory.
   3. Skip when the chosen topic has no memories in the top k, or when the best match already belongs to it.
   4. Reject when the best match beats the chosen topic's best memory by more than the margin. The margin is relative (0.3 means 30% higher), a constant for now, and comes from tuning (section 6).
   5. The rejection suggests the better topic (id, name, description) next to the chosen one.

#### Tools

1. **`create_topic(name, description, force?)`**
   1. Reject an empty name.
   2. Embed the name, outside the transaction.
   3. Topic name check.
   4. Insert, return the topic id.

2. **`update_topic(id, name?, description?, force?)`**
   1. Reject if the topic doesn't exist, or if neither name nor description is given.
   2. If the name changes: embed it outside the transaction, then run the topic name check.
   3. Update, return the topic id.

3. **`create_memory(topic_id, content, force?)`**
   1. Reject empty content, or content over 280 characters, with an error telling the model to shorten it.
   2. Embed the content, outside the transaction.
   3. Reject if the topic doesn't exist, returning the top topics from the memory check's grouped result so the model has something to retry with.
   4. Memory check.
   5. Fit score.
   6. Insert, return the memory id.

4. **`update_memory(id, content?, topic_id?, force?)`**
   1. Reject if the memory doesn't exist, or if neither content nor topic_id is given.
   2. Same content rules as `create_memory`.
   3. If the content changes: embed it outside the transaction, then run the memory check excluding this memory.
   4. If the topic changes: reject if the new topic doesn't exist.
   5. Fit score, whenever the content or the topic changes.
   6. Update, return the memory id.

#### Every write

1. **One transaction.** Embed before it opens, then run the checks and the write inside one write transaction, so two sessions can't both pass the check and insert the same fact.
2. **Descriptions are display only.** Shown in `get` and in injection, never embedded, never used for matching.
3. **Every write call is logged** as an `events` row (section 1, table 5). A thin wrapper around the service writes the row after the call returns, so a rejection gets logged without touching the write's transaction. Database triggers separately keep an audit trail of the actual row changes.
4. **Thresholds.** Memory threshold and topic threshold are both raw cosine and configurable. The 0.85 fallback gets replaced in section 6. Before building the rest, embed 10 to 15 hand picked pairs (same fact, same domain but different, negations) to check the fallback is anywhere near sane for GTE.

### 5. Interfaces

1. **`mem mcp`:** MCP server over stdin and stdout, using the official Go SDK. All logs go to stderr, because anything else printed to stdout corrupts the protocol. Tools:
   1. `create_topic(name, description, force?)`
   2. `update_memory(id, content?, topic_id?, force?)`
   3. `search(query)`: returns the top results, ungated (item 2 only applies to `inject`). Records what it returns in `injections` for the current session.
   4. `get_memory(id)`
   5. `get_topic(id)`: a topic id returns its description and all its facts. Records every fact it returns in `injections` for the current session, so a topic the agent has read in full isn't injected again. `mem mcp` knows the current session from the latest `inject` call.
   6. `delete_topic(id)`
   7. `delete_memory(id)`
   8. `update_topic(id, name?, description?, force?)`
   9. `create_memory(topic_id, content, force?)`
   10. `inject(prompt, session_id)`: called by the harness on every user prompt, never by the agent (its description says so). Searches the prompt, applies items 2 and 3, records what it returns in `injections`, and returns readable text (not JSON), because the harness pastes the result into the context as is.
   11. `clear_injected(session_id)`: called by the harness after a compaction. Deletes that session's `injections` rows, because compaction can drop earlier injections from the context while the session id stays the same.
2. **When to inject:** a fact gets injected only if it stands out from the noise for this prompt.
   1. Score every fact against the prompt, and take the median of all those scores as the baseline. Most facts are unrelated to any given prompt, so the median shows what "unrelated" looks like this time. Already-shown facts count towards it too.
   2. Drop the facts the agent has already seen in this session (item 3).
   3. Inject the remaining facts whose cosine clears a minimum and sits at least a gap above the baseline. Both numbers come from tuning (section 6).
   4. Why a gap above the baseline: GTE scores even unrelated text fairly high, so a vague prompt ("ok, fix that") can score evenly against many facts, and a minimum alone would let one creep over by chance. Comparing against the baseline instead of the second best still lets two equally strong matches through.
   5. With only a few facts, the median sits high and little gets through. That fixes itself as the store grows.
3. **At most a fixed number of entries,** best first. It's 5 for now, a constant in code. Each is a fact plus its topic (id, name, description, fact count). Several facts from the same topic can each be an entry.
   1. Whatever the agent has seen isn't injected again in the same session. That means facts returned by `inject`, by `search`, and every fact of a topic read with `get_topic`. All three record them in `injections` and count as a retrieval for usage decay (`last_retrieved_at`, `hit_count`).
4. **Tool descriptions are instructions for the agent:** write self-contained facts ("ob's PC budget is €2000", not "budget is 2000"), search before writing, and how to answer "did you mean".
5. **`mem export`:** prints every topic and its facts as markdown to stdout, on demand. Read-only, never changes anything. Useful as a readable backup and to diff over time in git.
   1. Format, per topic, with topics sorted by name and facts by `created_at`:
      ```
      # topic name
      ## topic description

      - fact 1
      - fact 2
      ```

6. **`mem tidy`:** the daily pass. In v1 it's pure code:
   1. Usage decay: a few if statements with threshold values ob picks by hand, kept in the same embedded JSON file as the tuned values (section 6). They look at how long since a fact or topic was last retrieved (last_retrieved_at) and how many times it was retrieved (hit_count). Never at how old something is, and never tuned automatically. Facts that decay marks as obsolete are hard-deleted.
   2. Topics left with no facts are deleted.
7. **Claude Code notes** (the wiring itself lives outside `mem`, per ground rule 3):
   1. A UserPromptSubmit hook of type `mcp_tool` calls `inject` with `{"prompt": "${prompt}", "session_id": "${session_id}"}`. The tool's text output is added to the context next to the prompt. The default timeout on UserPromptSubmit is 30 seconds, and a timeout or failure lets the prompt through without injection.
   2. A SessionStart hook with matcher `compact` calls `clear_injected` with `{"session_id": "${session_id}"}`.
   3. Observed `session_id` behaviour: the same for every prompt in a session; new on a fresh start and on `/clear`; kept on resume and on compaction.
   4. Headless runs (`claude -p`, like the SessionEnd transcript analyzer) fire the same hooks with input that looks exactly like an interactive session, so `mem` can't tell them apart. They must be ignored on the harness side, by starting them with hooks disabled (`--settings '{"disableAllHooks": true}'`).
   5. Still to verify once `inject` exists: that `${prompt}` and `${session_id}` are substituted in `mcp_tool` input on UserPromptSubmit (only command hooks were tested so far), and that the hook's `mcp_tool` call reaches the same `mem mcp` process the agent talks to, since `get_topic` relies on it for the session id.

### 6. Logging and tuning

1. **Log every query** in `events` from day one.
2. **Tuning** comes from a tool that is run by hand, like the benchmark. It never runs on start:
   1. An example set written by ob, with the correct answer for every example. It's built from ob's own notes and prompts, so it stays local and gitignored (`tuner/examples.json`):
      1. Duplicate, near duplicate, related and unrelated fact pairs, plus a few negation pairs. These tune the duplicate thresholds and the consolidation grouping threshold.
      2. Facts with prompts, and whether each prompt should inject them. These tune the injection minimum and gap.
      3. Facts and topics with queries and the results each should return. These tune search, topic resolution and the "did you mean" cutoff.
   2. The tool runs the set, picks the numbers that get the most examples right, and writes them to a JSON file in the repo. Changes show up in git diffs.
   3. The binary embeds that file (`go:embed`) and reads it once on start. A file that doesn't parse stops the start, and a test parses it so that's caught before shipping.
   4. Keep the number of tuned values small compared to the number of examples, and check the result on a part of the set that tuning didn't see. Otherwise the numbers just memorize the examples and fail on real data.
   5. The tool has integration tests of its own.
   6. `make tune` runs it. `go run ./tuner -diagnose` (with `gte_small_directory` set) prints each inject prompt's injected, missed and wrong facts at the current values, plus the silence rate, to review labels and results.
3. **Test queries,** for you during development: about 30 queries with their expected results. Run them after every ranking change so you notice when something gets worse.

## v2: continuous refinement

The goal: the daily pass keeps going over the memories and refines them. It merges duplicates, splits topics that grew too big, merges topics that drifted together, and finds groups held together by purpose rather than subject ("things to tell Jason"). Embeddings can't see purpose, so this needs a language model. Code does everything it can first, and the model reviews and fixes what code proposes. Reviewing a proposal is far easier for a small model than building groups from scratch. The model never deletes anything for being obsolete (ground rule 7).

1. **Built-in language model:** goinfer (a pure Go library that runs language models) with Gemma 4 E2B (Google's small model, Apache 2.0 license, about 1.5GB). Move to E4B (bigger, needs a few GB of RAM) only if E2B fails on the test cases.
   1. Download the weights on first run with a SHA-256 check instead of embedding them in the binary.
   2. Pin goinfer to a fixed version. It's a small project, not yet at version 1.0.
   3. Put it behind a small Go interface of your own so it can be swapped.
   4. Temperature 0 (the model always picks its most likely next word, so the same input gives the same output) and constrained output (the model can only produce text that matches a fixed JSON shape). Validate the result anyway.
2. **Never on a path anyone waits for.** On CPU in pure Go it will probably manage only a few words per second, so `mem mcp` never loads it, `inject` included. It only runs inside `mem tidy`.
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

1. Reading session transcripts (`mem extract`). The agent writing through the create and update tools has the full context and is a far better model.
2. Approximate nearest neighbour indexes, SQLite vector extensions, multilingual support, a separate background server, cloud.
3. Tuning or re-embedding automatically on start, and detecting model changes. The model and aikit stay frozen, and tuning is run by hand (section 6).
4. A checksum of the model weights. They're compiled into the binary.
