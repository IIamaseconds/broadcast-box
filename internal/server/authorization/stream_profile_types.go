package authorization

import (
	"strings"
)

// Internal Profile struct, do not use for endpoints
type Profile struct {
	FileName string
	IsActive bool
	IsPublic bool
	MOTD     string
}

var separator = "_"

func (p *Profile) StreamKey() string {
	splitIndex := strings.LastIndex(p.FileName, separator)
	return p.FileName[:splitIndex+len(separator)-1]
}
func (p *Profile) StreamToken() string {
	splitIndex := strings.LastIndex(p.FileName, separator)
	return p.FileName[splitIndex+len(separator):]
}
func (p *Profile) AsPublicProfile() *PublicProfile {
	return &PublicProfile{
		StreamKey: p.StreamKey(),
		IsActive:  p.IsActive,
		IsPublic:  p.IsPublic,
		MOTD:      p.MOTD,
	}
}
func (p *Profile) AsPersonalProfile() *PersonalProfile {
	return &PersonalProfile{
		StreamKey: p.StreamKey(),
		IsActive:  p.IsActive,
		IsPublic:  p.IsPublic,
		MOTD:      p.MOTD,
	}
}
func (p *Profile) AsAdminProfile() *AdminProfile {
	return &AdminProfile{
		StreamKey: p.StreamKey(),
		Token:     p.StreamToken(),
		IsPublic:  p.IsPublic,
		MOTD:      p.MOTD,
	}
}

// Public profile struct for serving to public endpoints
type PublicProfile struct {
	StreamKey string `json:"streamKey"`
	IsActive  bool   `json:"isActive"`
	IsPublic  bool   `json:"isPublic"`
	MOTD      string `json:"motd"`
}

// Personal profile struct for serving to profile owner endpoints
type PersonalProfile struct {
	StreamKey string `json:"streamKey"`
	IsActive  bool   `json:"isActive"`
	IsPublic  bool   `json:"isPublic"`
	MOTD      string `json:"motd"`
}

// Admin profile struct for serving to admin specific endpoints
type AdminProfile struct {
	StreamKey string `json:"streamKey"`
	Token     string `json:"token"`
	IsPublic  bool   `json:"isPublic"`
	MOTD      string `json:"motd"`
}
