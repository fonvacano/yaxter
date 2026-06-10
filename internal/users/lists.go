package users

import "context"

type Summary struct {
	ID        int64
	Username  string
	AvatarKey *string
}

// Followers pages through who follows username, newest-id first.
func (s *Service) Followers(ctx context.Context, username string, cursor int64, limit int) ([]Summary, int64, error) {
	return s.edgePage(ctx, username, cursor, limit, `
		SELECT u.id, u.username, u.avatar_key
		FROM followers f JOIN users u ON u.id = f.follower_id
		WHERE f.followee_id = $1 AND ($2 = 0 OR f.follower_id < $2)
		ORDER BY f.follower_id DESC LIMIT $3`)
}

// Following pages through who username follows.
func (s *Service) Following(ctx context.Context, username string, cursor int64, limit int) ([]Summary, int64, error) {
	return s.edgePage(ctx, username, cursor, limit, `
		SELECT u.id, u.username, u.avatar_key
		FROM follows f JOIN users u ON u.id = f.followee_id
		WHERE f.follower_id = $1 AND ($2 = 0 OR f.followee_id < $2)
		ORDER BY f.followee_id DESC LIMIT $3`)
}

func (s *Service) edgePage(ctx context.Context, username string, cursor int64, limit int, query string) ([]Summary, int64, error) {
	owner, err := s.GetByUsername(ctx, username)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(ctx, query, owner.ID, cursor, limit+1)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		var u Summary
		if err := rows.Scan(&u.ID, &u.Username, &u.AvatarKey); err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var next int64
	if len(out) > limit {
		out = out[:limit]
		next = out[limit-1].ID
	}
	return out, next, nil
}
