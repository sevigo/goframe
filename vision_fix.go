// goframe/agent/loop.go vision support fix
// Replace lines 353-374 in actAndObserve() function

// Create observation message - check for images first (vision support)
if err != nil {
    l.logger.Error("tool execution failed",
        "tool", toolName,
        "error", err,
    )
    obsContent := fmt.Sprintf("Tool '%s' failed: %s", toolName, err.Error())
    observations = append(observations, schema.NewToolResultMessage(toolName, obsContent))
} else {
    l.logger.Debug("tool execution succeeded", "tool", toolName)

    // Check if result contains image for vision models
    if resultMap, ok := result.(map[string]any); ok {
        var imageData string
        var found bool

        // Check various image field names
        if img, ok := resultMap["imageBase64"].(string); ok && img != "" {
            imageData = img
            found = true
        } else if img, ok := resultMap["image"].(string); ok && img != "" {
            imageData = img
            found = true
        }

        // If image found, create multimodal message for vision models
        if found && len(imageData) > 100 {
            textPart := schema.TextContent{Text: fmt.Sprintf("Tool '%s' returned (see image):", toolName)}
            imagePart := schema.ImageContent{Data: imageData, MimeType: "image/png"}

            obsMsg := schema.MessageContent{
                Role:  schema.ChatMessageTypeTool,
                Parts: []schema.ContentPart{textPart, imagePart},
            }
            observations = append(observations, obsMsg)
            continue
        }
    }

    // Default: serialize result to JSON text
    jsonBytes, jsonErr := json.Marshal(result)
    var obsContent string
    if jsonErr != nil {
        obsContent = fmt.Sprintf("Tool '%s' returned: %v", toolName, result)
    } else {
        obsContent = fmt.Sprintf("Tool '%s' returned: %s", toolName, string(jsonBytes))
    }
    observations = append(observations, schema.NewToolResultMessage(toolName, obsContent))
}