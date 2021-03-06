package store

import (
	"API_server/api/rethink"
	"API_server/dbscript"
	"API_server/domain"
	"encoding/json"
)

type Store struct {
	re *rethink.Instance
}

func NewStore(re *rethink.Instance) *Store {
	return &Store{
		re: re,
	}
}

func (this *Store) ListAllUser() (users []*domain.User) {
	this.re.All(this.re.Table(domain.USER_TABLE), &users)
	return
}

func (this *Store) CreateUser(user *domain.User) error {
	_, err := this.re.RunWrite(this.re.Table(domain.USER_TABLE).Insert(user))
	return err
}

func (this *Store) GetUser(id string) (user *domain.User, err error) {
	err = this.re.One(this.re.Table(domain.USER_TABLE).Get(id), &user)
	return
}

func (this *Store) StartMatch(players [2]string, matchid string) error {
	for _, userid := range players {
		user, err := this.GetUser(userid)
		if err != nil {
			return err
		}
		user.Match = append([]string{matchid}, user.Match...)
		_, err = this.re.RunWrite(this.re.Table(domain.USER_TABLE).Get(userid).Update(user))
		if err != nil {
			return err
		}
	}

	return nil
}

func (this *Store) UpdateWin(userid string) error {
	user, err := this.GetUser(userid)
	if err != nil {
		return err
	}
	user.Win++
	_, err = this.re.RunWrite(this.re.Table(domain.USER_TABLE).Get(userid).Update(user))
	return err
}

func (this *Store) ListAllMatch() (match []*domain.Match) {
	this.re.All(this.re.Table(domain.MATCH_TABLE), &match)
	return
}

func (this *Store) CreateMatch(match *domain.Match) error {
	rw, err := this.re.RunWrite(this.re.Table(domain.MATCH_TABLE).Insert(match))
	if err != nil {
		return err
	}
	match.Id = rw.GeneratedKeys[0]
	return nil
}

func (this *Store) UpdateMatch(match *domain.Match) error {
	_, err := this.re.RunWrite(this.re.Table(domain.MATCH_TABLE).Get(match.Id).Update(match))
	return err
}

func (this *Store) GetMatch(matchid string) (match *domain.Match, err error) {
	err = this.re.One(this.re.Table(domain.MATCH_TABLE).Get(matchid), &match)
	return
}

func (this *Store) GetUserRank(userId string) int {
	cursor, err := this.re.Run(this.re.OrderByDesc(this.re.Table(domain.USER_TABLE), dbscript.WIN_INDEX))
	if err != nil {
		return -1
	}
	var user *domain.User
	defer cursor.Close()
	for i := 1; cursor.Next(&user); i++ {
		if userId == user.Id {
			return i
		}
	}
	return -1
}

func (this *Store) GetAverageTurn(userId string, matches []string) (averageTurn int, err error) {
	if len(matches) == 0 {
		return 0, nil
	}
	totalTurn := 0
	for _, mId := range matches {
		match, err := this.GetMatch(mId)
		if err != nil {
			return 0, err
		}
		totalTurn = totalTurn + match.Turn
	}
	averageTurn = totalTurn / len(matches)
	return
}

func (this *Store) GetProfile(userId string, matchLimit int) ([]byte, error) {
	user, err := this.GetUser(userId)
	if err != nil {
		return nil, err
	}
	win := user.Win
	averageTurn, err := this.GetAverageTurn(userId, user.Match)
	if err != nil {
		return nil, err
	}
	matches := []*domain.Match{}
	for _, mId := range user.Match {
		match, err := this.GetMatch(mId)
		if err != nil {
			return nil, err
		}
		matches = append(matches, match)
	}
	type MatchResponse struct {
		Id       string `json:"id"`
		Opponent string `json:"opponent"`
		Winner   string `json:"winner"`
		Turn     int    `json:"turn"`
		Time     string `json:"time"`
	}
	type ProfileResponse struct {
		Id          string           `json:"id"`
		Name        string           `json:"name"`
		FBprofile   string           `json:"fbprofile"`
		TotalMatch  int              `json:"totalmatch"`
		Win         int              `json:"win"`
		Rank        int              `json:"rank"`
		AverageTurn int              `json:"averageturn"`
		Matches     []*MatchResponse `json:"matches"`
	}
	matchResponse := []*MatchResponse{}
	for _, match := range matches {
		if len(matchResponse) == matchLimit {
			break
		}
		var opponent string
		if userId == match.Player[0] {
			opponent = match.Player[1]
		} else {
			opponent = match.Player[0]
		}
		mr := &MatchResponse{
			Id:       match.Id,
			Opponent: opponent,
			Winner:   match.Winner,
			Turn:     match.Turn,
			Time:     match.CreatedTime.Format("15:04, _2-Jan-2006"),
		}
		matchResponse = append(matchResponse, mr)
	}
	response := &ProfileResponse{
		Id:          user.Id,
		Name:        user.Name,
		FBprofile:   "https://facebook.com/" + user.Id,
		TotalMatch:  len(user.Match),
		Rank:        this.GetUserRank(userId),
		Win:         win,
		AverageTurn: averageTurn,
		Matches:     matchResponse,
	}
	p, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (this *Store) GetRanking(limit int) ([]byte, error) {
	type PlayerResponse struct {
		Id          string `json:"id"`
		Name        string `json:"name"`
		TotalMatch  int    `json:"totalmatch"`
		Win         int    `json:"win"`
		AverageTurn int    `json:"averageturn"`
		Rank        int    `json:"rank"`
	}
	var response []*PlayerResponse
	var user domain.User
	cursor, err := this.re.Run(this.re.OrderByDesc(this.re.Table(domain.USER_TABLE), dbscript.WIN_INDEX), limit)
	if err != nil {
		return nil, err
	}
	defer cursor.Close()
	for i := 1; cursor.Next(&user); i++ {
		p := &PlayerResponse{
			Id:         user.Id,
			Name:       user.Name,
			TotalMatch: len(user.Match),
			Win:        user.Win,
		}
		avgTurn, err := this.GetAverageTurn(user.Id, user.Match)
		if err != nil {
			p.AverageTurn = -1
		}
		p.AverageTurn = avgTurn
		p.Rank = i
		response = append(response, p)
	}
	res, err := json.Marshal(response)
	return res, err
}

func (this *Store) GetMatchInfo(matchId string) ([]byte, error) {
	type PlayerResponse struct {
		Id   string `json:"id"`
		Name string `json:"name"`
	}
	type MatchResponse struct {
		Id     string             `json:"id"`
		Player [2]*PlayerResponse `json:"player"`
		Winner *PlayerResponse    `json:"winner"`
		Turn   int                `json:"turn"`
		Time   string             `json:"time"`
	}
	match, err := this.GetMatch(matchId)
	if err != nil {
		return nil, err
	}
	var player [2]*PlayerResponse
	var winner *PlayerResponse
	for i, id := range match.Player {
		p, err := this.GetUser(id)
		if err != nil {
			return nil, err
		}
		player[i] = &PlayerResponse{
			Id:   p.Id,
			Name: p.Name,
		}
		if match.Winner == id {
			winner = player[i]
		}
	}
	matchResponse := &MatchResponse{
		Id:     match.Id,
		Player: player,
		Winner: winner,
		Turn:   match.Turn,
		Time:   match.CreatedTime.Format("15:04, _2-Jan-2006"),
	}
	res, err := json.Marshal(matchResponse)
	return res, err
}
