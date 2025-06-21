package prompts

// DefaultRAGPrompt is a default prompt for Retrieval-Augmented Generation.
var DefaultRAGPrompt = NewPromptTemplate(
	`Use the following context to answer the question at the end.
If you don't know the answer, just say that you don't know, don't try to make up an answer.

Context:
{{.context}}

Question: {{.query}}

Helpful Answer:`)
