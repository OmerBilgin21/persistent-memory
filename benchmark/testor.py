import json

from sentence_transformers import SentenceTransformer

with open("benchmark-data.json") as f:
    sentences = json.load(f)["texts"]

m = SentenceTransformer("thenlper/gte-small")
vecs = m.encode(sentences, normalize_embeddings=True)

# Idk, just dump it as reference.json
with open("reference.json", "w") as f:
    json.dump([{"text": s, "vec": v.tolist()} for s, v in zip(sentences, vecs)], f)
