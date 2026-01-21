package gdocs

import (
	"context"
	"fmt"

	"bauer/internal/models"
)

// FetchComments fetches all comments from the document using Drive API.
func (c *Client) FetchComments(ctx context.Context, docID string) ([]models.Comment, error) {
	var comments []models.Comment
	pageToken := ""

	for {
		req := c.Drive.Comments.List(docID).
			Fields("nextPageToken, comments(id, author(displayName, emailAddress), content, quotedFileContent, createdTime, modifiedTime, resolved, replies(id, author(displayName, emailAddress), content, createdTime), mentionedEmailAddresses, anchor)").
			Context(ctx)

		if pageToken != "" {
			req = req.PageToken(pageToken)
		}

		resp, err := req.Do()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch comments: %w", err)
		}

		for _, c := range resp.Comments {
			comment := models.Comment{
				ID:           c.Id,
				Content:      c.Content,
				CreatedTime:  c.CreatedTime,
				ModifiedTime: c.ModifiedTime,
				Resolved:     c.Resolved,
			}

			if c.Author != nil {
				comment.Author = c.Author.DisplayName
				comment.AuthorEmail = c.Author.EmailAddress
			}

			if c.QuotedFileContent != nil {
				comment.QuotedContent = c.QuotedFileContent.Value
			}

			for _, r := range c.Replies {
				reply := models.Reply{
					ID:          r.Id,
					Content:     r.Content,
					CreatedTime: r.CreatedTime,
				}
				if r.Author != nil {
					reply.Author = r.Author.DisplayName
					reply.AuthorEmail = r.Author.EmailAddress
				}
				comment.Replies = append(comment.Replies, reply)
			}

			comments = append(comments, comment)
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return comments, nil
}
