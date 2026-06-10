// Package httpapi hosts the generated OpenAPI server. Server implements the
// full generated ServerInterface; each domain module fills in its methods as
// its task lands, the rest return 501.
package httpapi

import (
	"net/http"

	"github.com/fonvacano/yaxter/internal/auth"
	"github.com/fonvacano/yaxter/internal/auth/oauth"
	"github.com/fonvacano/yaxter/internal/media"
	"github.com/fonvacano/yaxter/internal/tweets"
	"github.com/fonvacano/yaxter/internal/users"
)

type Server struct {
	Auth   *AuthHandlers
	Users  *UsersHandlers
	Tweets *TweetsHandlers
	Media  *MediaHandlers
	OAuth  *OAuthHandlers
}

func NewServer(authSvc *auth.Service, usersSvc *users.Service, mediaBaseURL string, tweetsSvc *tweets.Service, mediaSvc *media.Service, oauthFlow *oauth.Flow) *Server {
	return &Server{
		Auth:   &AuthHandlers{svc: authSvc},
		Users:  &UsersHandlers{svc: usersSvc, mediaBaseURL: mediaBaseURL},
		Tweets: &TweetsHandlers{svc: tweetsSvc},
		Media:  &MediaHandlers{svc: mediaSvc, mediaBaseURL: mediaBaseURL},
		OAuth:  &OAuthHandlers{flow: oauthFlow, svc: authSvc},
	}
}

var _ ServerInterface = (*Server)(nil)

// ---- implemented: auth (T1.1) ----

func (s *Server) Register(w http.ResponseWriter, r *http.Request, params RegisterParams) {
	s.Auth.Register(w, r)
}
func (s *Server) Login(w http.ResponseWriter, r *http.Request)        { s.Auth.Login(w, r) }
func (s *Server) RefreshToken(w http.ResponseWriter, r *http.Request) { s.Auth.Refresh(w, r) }
func (s *Server) Logout(w http.ResponseWriter, r *http.Request)       { s.Auth.Logout(w, r) }

// ---- 501 until their tasks land ----

func (s *Server) ListAuthProviders(w http.ResponseWriter, r *http.Request) {
	s.OAuth.ListProviders(w, r)
}
func (s *Server) OauthCallback(w http.ResponseWriter, r *http.Request, provider OauthCallbackParamsProvider, params OauthCallbackParams) {
	s.OAuth.Callback(w, r, string(provider))
}
func (s *Server) OauthUnlink(w http.ResponseWriter, r *http.Request, provider OauthUnlinkParamsProvider) {
	s.OAuth.Unlink(w, r, string(provider))
}
func (s *Server) OauthLink(w http.ResponseWriter, r *http.Request, provider OauthLinkParamsProvider) {
	s.OAuth.Link(w, r, string(provider))
}
func (s *Server) OauthStart(w http.ResponseWriter, r *http.Request, provider OauthStartParamsProvider, params OauthStartParams) {
	s.OAuth.Start(w, r, string(provider))
}

func (s *Server) GetMe(w http.ResponseWriter, r *http.Request) { s.Users.GetMe(w, r) }
func (s *Server) UpdateMe(w http.ResponseWriter, r *http.Request, params UpdateMeParams) {
	s.Users.UpdateMe(w, r)
}
func (s *Server) GetUser(w http.ResponseWriter, r *http.Request, username Username) {
	s.Users.GetUser(w, r, username)
}
func (s *Server) UnfollowUser(w http.ResponseWriter, r *http.Request, username Username, params UnfollowUserParams) {
	s.Users.UnfollowUser(w, r, username)
}
func (s *Server) FollowUser(w http.ResponseWriter, r *http.Request, username Username, params FollowUserParams) {
	s.Users.FollowUser(w, r, username)
}
func (s *Server) ListFollowers(w http.ResponseWriter, r *http.Request, username Username, params ListFollowersParams) {
	s.Users.ListFollowers(w, r, username, params)
}
func (s *Server) ListFollowing(w http.ResponseWriter, r *http.Request, username Username, params ListFollowingParams) {
	s.Users.ListFollowing(w, r, username, params)
}
func (s *Server) GetUserTweets(w http.ResponseWriter, r *http.Request, username Username, params GetUserTweetsParams) {
	unimplemented(w) // T2.4
}

func (s *Server) CreateTweet(w http.ResponseWriter, r *http.Request, params CreateTweetParams) {
	s.Tweets.Create(w, r)
}
func (s *Server) DeleteTweet(w http.ResponseWriter, r *http.Request, id TweetId) {
	s.Tweets.Delete(w, r, id)
}
func (s *Server) GetTweet(w http.ResponseWriter, r *http.Request, id TweetId) {
	s.Tweets.Get(w, r, id)
}
func (s *Server) UnlikeTweet(w http.ResponseWriter, r *http.Request, id TweetId, params UnlikeTweetParams) {
	if s.Tweets == nil {
		unimplemented(w)
		return
	}
	s.Tweets.setLike(w, r, id, false)
}
func (s *Server) LikeTweet(w http.ResponseWriter, r *http.Request, id TweetId, params LikeTweetParams) {
	if s.Tweets == nil {
		unimplemented(w)
		return
	}
	s.Tweets.setLike(w, r, id, true)
}

func (s *Server) GetHomeTimeline(w http.ResponseWriter, r *http.Request, params GetHomeTimelineParams) {
	unimplemented(w) // T2.4
}

func (s *Server) CreateMedia(w http.ResponseWriter, r *http.Request, params CreateMediaParams) {
	s.Media.Create(w, r)
}
func (s *Server) GetMedia(w http.ResponseWriter, r *http.Request, id MediaId) {
	s.Media.Get(w, r, string(id))
}
func (s *Server) CompleteMedia(w http.ResponseWriter, r *http.Request, id MediaId) {
	s.Media.Complete(w, r, string(id))
}

func (s *Server) ListNotifications(w http.ResponseWriter, r *http.Request, params ListNotificationsParams) {
	unimplemented(w) // T2.3
}
func (s *Server) MarkNotificationsRead(w http.ResponseWriter, r *http.Request, params MarkNotificationsReadParams) {
	unimplemented(w) // T2.3
}
func (s *Server) GetUnreadCount(w http.ResponseWriter, r *http.Request) { unimplemented(w) } // T2.3
