// Package httpapi hosts the generated OpenAPI server. Server implements the
// full generated ServerInterface; each domain module fills in its methods as
// its task lands, the rest return 501.
package httpapi

import (
	"net/http"

	"github.com/fonvacano/yaxter/internal/auth"
	"github.com/fonvacano/yaxter/internal/tweets"
)

type Server struct {
	Auth   *AuthHandlers
	Tweets *TweetsHandlers
}

func NewServer(authSvc *auth.Service, tweetsSvc *tweets.Service) *Server {
	return &Server{
		Auth:   &AuthHandlers{svc: authSvc},
		Tweets: &TweetsHandlers{svc: tweetsSvc},
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

func (s *Server) ListAuthProviders(w http.ResponseWriter, r *http.Request) { unimplemented(w) } // T1.6
func (s *Server) OauthCallback(w http.ResponseWriter, r *http.Request, provider OauthCallbackParamsProvider, params OauthCallbackParams) {
	unimplemented(w) // T1.6
}
func (s *Server) OauthUnlink(w http.ResponseWriter, r *http.Request, provider OauthUnlinkParamsProvider) {
	unimplemented(w) // T1.6
}
func (s *Server) OauthLink(w http.ResponseWriter, r *http.Request, provider OauthLinkParamsProvider) {
	unimplemented(w) // T1.6
}
func (s *Server) OauthStart(w http.ResponseWriter, r *http.Request, provider OauthStartParamsProvider, params OauthStartParams) {
	unimplemented(w) // T1.6
}

func (s *Server) GetMe(w http.ResponseWriter, r *http.Request) { unimplemented(w) } // T1.2
func (s *Server) UpdateMe(w http.ResponseWriter, r *http.Request, params UpdateMeParams) {
	unimplemented(w) // T1.2
}
func (s *Server) GetUser(w http.ResponseWriter, r *http.Request, username Username) {
	unimplemented(w) // T1.2
}
func (s *Server) UnfollowUser(w http.ResponseWriter, r *http.Request, username Username, params UnfollowUserParams) {
	unimplemented(w) // T1.2
}
func (s *Server) FollowUser(w http.ResponseWriter, r *http.Request, username Username, params FollowUserParams) {
	unimplemented(w) // T1.2
}
func (s *Server) ListFollowers(w http.ResponseWriter, r *http.Request, username Username, params ListFollowersParams) {
	unimplemented(w) // T1.2
}
func (s *Server) ListFollowing(w http.ResponseWriter, r *http.Request, username Username, params ListFollowingParams) {
	unimplemented(w) // T1.2
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
	unimplemented(w) // T1.3
}
func (s *Server) LikeTweet(w http.ResponseWriter, r *http.Request, id TweetId, params LikeTweetParams) {
	unimplemented(w) // T1.3
}

func (s *Server) GetHomeTimeline(w http.ResponseWriter, r *http.Request, params GetHomeTimelineParams) {
	unimplemented(w) // T2.4
}

func (s *Server) CreateMedia(w http.ResponseWriter, r *http.Request, params CreateMediaParams) {
	unimplemented(w) // T1.5
}
func (s *Server) GetMedia(w http.ResponseWriter, r *http.Request, id MediaId) {
	unimplemented(w) // T1.5
}
func (s *Server) CompleteMedia(w http.ResponseWriter, r *http.Request, id MediaId) {
	unimplemented(w) // T1.5
}

func (s *Server) ListNotifications(w http.ResponseWriter, r *http.Request, params ListNotificationsParams) {
	unimplemented(w) // T2.3
}
func (s *Server) MarkNotificationsRead(w http.ResponseWriter, r *http.Request, params MarkNotificationsReadParams) {
	unimplemented(w) // T2.3
}
func (s *Server) GetUnreadCount(w http.ResponseWriter, r *http.Request) { unimplemented(w) } // T2.3
