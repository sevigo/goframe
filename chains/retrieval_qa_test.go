package chains_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/chains"
	"github.com/sevigo/goframe/llms/fake"
	"github.com/sevigo/goframe/schema"
	fakeretriever "github.com/sevigo/goframe/schema/fake"
)

func TestRetrievalQA_Call(t *testing.T) {
	ctx := context.Background()

	t.Run("Success with documents", func(t *testing.T) {
		retrievedDocs := []schema.Document{
			{PageContent: "The sky is blue."},
			{PageContent: "Grass is green."},
		}

		docContents := []string{"The sky is blue.", "Grass is green."}
		contextStr := strings.Join(docContents, "\n\n---\n\n")
		expectedPrompt := fmt.Sprintf(`Use the following context to answer the question at the end.
If you don't know the answer, just say that you don't know, don't try to make up an answer.

Context:
%s

Question: What colors are in nature?

Helpful Answer:`, contextStr)

		fakeLLM := fake.NewFakeLLM([]string{"Blue and green are colors in nature."})
		fakeRetriever := fakeretriever.NewRetriever()
		fakeRetriever.DocsToReturn = retrievedDocs

		ragChain, err := chains.NewRetrievalQA(fakeRetriever, fakeLLM)
		require.NoError(t, err)

		answer, err := ragChain.Call(ctx, "What colors are in nature?")

		require.NoError(t, err)
		assert.Equal(t, "Blue and green are colors in nature.", answer)

		lastPrompt, _ := fakeLLM.LastPrompt()
		assert.Equal(t, expectedPrompt, lastPrompt)
	})

	t.Run("Fallback when no documents are found", func(t *testing.T) {
		fakeLLM := fake.NewFakeLLM([]string{"I'm not sure, I have no context."})
		fakeRetriever := fakeretriever.NewRetriever()
		fakeRetriever.DocsToReturn = []schema.Document{} // No documents found

		ragChain, err := chains.NewRetrievalQA(fakeRetriever, fakeLLM)
		require.NoError(t, err)

		answer, err := ragChain.Call(ctx, "A question with no context.")

		require.NoError(t, err)
		assert.Equal(t, "I'm not sure, I have no context.", answer)

		lastPrompt, _ := fakeLLM.LastPrompt()
		assert.Equal(t, "A question with no context.", lastPrompt)
	})

	t.Run("Error during document retrieval", func(t *testing.T) {
		retrievalErr := errors.New("database connection failed")
		fakeLLM := fake.NewFakeLLM([]string{})
		fakeRetriever := fakeretriever.NewRetriever()
		fakeRetriever.ErrToReturn = retrievalErr

		ragChain, err := chains.NewRetrievalQA(fakeRetriever, fakeLLM)
		require.NoError(t, err)
		_, err = ragChain.Call(ctx, "Any question.")

		require.Error(t, err)
		assert.ErrorIs(t, err, retrievalErr)
		assert.Contains(t, err.Error(), "document retrieval failed")

		assert.Equal(t, 0, fakeLLM.GetCallCount(), "LLM should not have been called when retrieval fails")
	})

	t.Run("Custom PromptBuilder is used", func(t *testing.T) {
		retrievedDocs := []schema.Document{
			{PageContent: "Go is compiled."},
		}

		fakeLLM := fake.NewFakeLLM([]string{"custom response"})
		fakeRetriever := fakeretriever.NewRetriever()
		fakeRetriever.DocsToReturn = retrievedDocs

		customBuilder := func(query string, docs []schema.Document) (string, error) {
			return fmt.Sprintf("CUSTOM: %s | docs=%d", query, len(docs)), nil
		}

		ragChain, err := chains.NewRetrievalQA(fakeRetriever, fakeLLM, chains.WithPromptBuilder(customBuilder))
		require.NoError(t, err)

		answer, err := ragChain.Call(ctx, "test query")

		require.NoError(t, err)
		assert.Equal(t, "custom response", answer)

		lastPrompt, _ := fakeLLM.LastPrompt()
		assert.Equal(t, "CUSTOM: test query | docs=1", lastPrompt)
	})
}
