package prompts

// DefaultRAGPrompt is a default prompt template for Retrieval-Augmented Generation.
// It instructs the model to answer based on the provided context and to acknowledge
// when it doesn't know the answer.
var DefaultRAGPrompt = NewPromptTemplate(
	`Use the following context to answer the question at the end.
If you don't know the answer, just say that you don't know, don't try to make up an answer.

Context:
{{.context}}

Question: {{.query}}

Helpful Answer:`)