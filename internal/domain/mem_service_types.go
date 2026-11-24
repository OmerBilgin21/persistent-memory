package domain

type Result interface{ result() }

func (UnexpectedErrorResult) result()      {}
func (CreatedMemoryResult) result()        {}
func (UpdatedMemoryResult) result()        {}
func (RejectedMemoryUpsertResult) result() {}

type UnexpectedErrorResult struct {
	Err error
}

type CreatedMemoryResult struct {
	TopicId          uint   `json:"topic_id"`
	TopicName        string `json:"topic_name"`
	TopicDescription string `json:"topic_description"`
	MemoryContent    string `json:"memory_content"`
}

type UpdatedMemoryResult struct {
	TopicId          uint   `json:"topic_id"`
	Content          string `json:"content"`
	Id               uint   `json:"id"`
	TopicName        string `json:"topic_name"`
	TopicDescription string `json:"topic_description"`
}

type RejectedMemoryUpsertResult struct {
	Reason string `json:"reason"`
}

type SimilarityTopic struct {
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	Id              uint    `json:"id"`
	CosineRank      float32 `json:"cosine_rank"`
	RRFRank         float32 `json:"rrf_rank"`
	DidBM25EffectIt bool    `json:"did_bm_25_effect_it"`
}

type SimilarityTopicWithMemories struct {
	SimilarityTopic
	Memories []SimilarityMemory `json:"memories"`
}

type SimilarityMemory struct {
	TopicId         uint    `json:"topic_id"`
	Content         string  `json:"content"`
	Id              uint    `json:"id"`
	CosineRank      float32 `json:"cosine_rank"`
	RRFRank         float32 `json:"rrf_rank"`
	DidBM25EffectIt bool    `json:"did_bm_25_effect_it"`
}

type SimilarityMemoryIndex map[uint]SimilarityMemory

type SimilarityResult struct {
	Topics   []SimilarityTopic
	Memories []SimilarityMemory
	Err      error
}

type CreateMemory struct {
	Fact    string `json:"fact"`
	Force   bool   `json:"force"`
	TopicId uint   `json:"topic_id"`
}

type UpdateMemory struct {
	Fact     *string `json:"fact"`
	TopicId  *uint   `json:"topic_id"`
	Force    bool    `json:"force"`
	MemoryId uint    `json:"memory_id"`
}

type CreateTopic struct {
	Name        string `json:"name"`
	Force       bool   `json:"force"`
	Description string `json:"description"`
}

type UpdateTopic struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	TopicId     uint    `json:"topic_id"`
	Force       bool    `json:"force"`
}
