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
		// Arrange
		retrievedDocs := []schema.Document{
			{PageContent: "The sky is blue."},
			{PageContent: "Grass is green."},
		}

		// Configure the fake LLM to expect a prompt with the context from the retrieved docs.
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

		ragChain := chains.NewRetrievalQA(fakeRetriever, fakeLLM)

		// Act
		answer, err := ragChain.Call(ctx, "What colors are in nature?")

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "Blue and green are colors in nature.", answer)

		// Verify the LLM was called with the correct, context-aware prompt.
		lastPrompt, _ := fakeLLM.LastPrompt()
		assert.Equal(t, expectedPrompt, lastPrompt)
	})

	t.Run("Fallback when no documents are found", func(t *testing.T) {
		// Arrange
		// Configure the LLM to respond to a direct query, as no context will be added.
		fakeLLM := fake.NewFakeLLM([]string{"I'm not sure, I have no context."})
		fakeRetriever := fakeretriever.NewRetriever()
		fakeRetriever.DocsToReturn = []schema.Document{} // No documents found

		ragChain := chains.NewRetrievalQA(fakeRetriever, fakeLLM)

		// Act
		answer, err := ragChain.Call(ctx, "A question with no context.")

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "I'm not sure, I have no context.", answer)

		// Verify the LLM was called with just the original query.
		lastPrompt, _ := fakeLLM.LastPrompt()
		assert.Equal(t, "A question with no context.", lastPrompt)
	})

	t.Run("Error during document retrieval", func(t *testing.T) {
		// Arrange
		retrievalErr := errors.New("database connection failed")
		fakeLLM := fake.NewFakeLLM([]string{}) // LLM should not be called
		fakeRetriever := fakeretriever.NewRetriever()
		fakeRetriever.ErrToReturn = retrievalErr

		ragChain := chains.NewRetrievalQA(fakeRetriever, fakeLLM)

		// Act
		_, err := ragChain.Call(ctx, "Any question.")

		// Assert
		require.Error(t, err)
		assert.ErrorIs(t, err, retrievalErr)
		assert.Contains(t, err.Error(), "document retrieval failed")

		// Verify the LLM was never called.
		assert.Equal(t, 0, fakeLLM.GetCallCount(), "LLM should not have been called when retrieval fails")
	})
}
