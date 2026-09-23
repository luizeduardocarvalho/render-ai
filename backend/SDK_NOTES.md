# Verified genai Go SDK notes (google.golang.org/genai v1.71.0)

These were read directly from the pinned SDK source. Use them; do not guess.

## Client (Vertex AI + ADC)
```go
client, err := genai.NewClient(ctx, &genai.ClientConfig{
    Backend:  genai.BackendVertexAI,
    Project:  cfg.Project,   // or env GOOGLE_CLOUD_PROJECT
    Location: "global",      // image models require global; or env GOOGLE_CLOUD_LOCATION
    // Credentials omitted => Application Default Credentials.
})
```

## Generate image
```go
resp, err := client.Models.GenerateContent(ctx, modelID, contents, &genai.GenerateContentConfig{
    ResponseModalities: []string{"IMAGE", "TEXT"}, // []string, not typed enum, on this struct
    ImageConfig: &genai.ImageConfig{
        AspectRatio: "16:9",  // "1:1","2:3","3:2","3:4","4:3","9:16","16:9","21:9"
        ImageSize:   "2K",    // "1K","2K","4K" (flash: "1K" only)
    },
    // Temperature *float32 and Seed *int32 exist on the struct but the IMAGE models do not
    // honor them meaningfully; leave both nil for renders.
})
```

## Build multi-image content
```go
parts := []*genai.Part{
    genai.NewPartFromBytes(screenshotPNG, "image/png"),
    genai.NewPartFromBytes(regionMapPNG, "image/png"),
    genai.NewPartFromBytes(edgeMapPNG,   "image/png"),
    // ... anchor, asset refs ...
    genai.NewPartFromText(promptText),
}
contents := []*genai.Content{ genai.NewContentFromParts(parts, genai.RoleUser) }
```

## Extract returned image bytes
```go
for _, cand := range resp.Candidates {
    for _, p := range cand.Content.Parts {
        if p.InlineData != nil { // *genai.Blob: .MIMEType, .Data []byte
            out = p.InlineData.Data
        }
    }
}
```
If no candidate contains InlineData, treat as "model returned no image" (surface a clear error;
also check finish reason / any text part for a refusal message).

## Text model (inventory + preservation check)
Same `GenerateContent` call with the text model id and no ImageConfig; read text from
`resp.Text()` (helper) or by concatenating `p.Text` parts. For the preservation inventory
check, ask for strict JSON and parse it.

## Token usage
`resp.UsageMetadata` carries prompt/candidate token counts when returned - use for metrics.
