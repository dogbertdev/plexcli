package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/LukeHagar/plexgo"
	"github.com/LukeHagar/plexgo/models/components"
	"github.com/LukeHagar/plexgo/models/operations"
)

type TokenAccountInfo struct {
	ID                 int      `json:"id"`
	Title              string   `json:"title"`
	Username           string   `json:"username"`
	Email              string   `json:"email"`
	FriendlyName       string   `json:"friendly_name"`
	UUID               string   `json:"uuid"`
	Home               bool     `json:"home"`
	HomeAdmin          bool     `json:"home_admin"`
	Guest              bool     `json:"guest"`
	Restricted         bool     `json:"restricted"`
	Confirmed          bool     `json:"confirmed"`
	Anonymous          bool     `json:"anonymous"`
	HasPassword        bool     `json:"has_password"`
	TwoFactorEnabled   bool     `json:"two_factor_enabled"`
	SubscriptionActive bool     `json:"subscription_active"`
	SubscriptionStatus string   `json:"subscription_status"`
	SubscriptionPlan   string   `json:"subscription_plan"`
	Roles              []string `json:"roles,omitempty"`
	Entitlements       []string `json:"entitlements,omitempty"`
}

type TokenConnectionInfo struct {
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	URI      string `json:"uri"`
	Local    bool   `json:"local"`
	Relay    bool   `json:"relay"`
	IPv6     bool   `json:"ipv6"`
}

type TokenResourceInfo struct {
	Name                   string                `json:"name"`
	Product                string                `json:"product"`
	ProductVersion         string                `json:"product_version"`
	Platform               string                `json:"platform"`
	PlatformVersion        string                `json:"platform_version"`
	Device                 string                `json:"device"`
	ClientIdentifier       string                `json:"client_identifier"`
	Provides               string                `json:"provides"`
	PublicAddress          string                `json:"public_address"`
	AccessToken            string                `json:"access_token"`
	Owned                  bool                  `json:"owned"`
	Home                   bool                  `json:"home"`
	Synced                 bool                  `json:"synced"`
	Relay                  bool                  `json:"relay"`
	Presence               bool                  `json:"presence"`
	HTTPSRequired          bool                  `json:"https_required"`
	PublicAddressMatches   bool                  `json:"public_address_matches"`
	DNSRebindingProtection bool                  `json:"dns_rebinding_protection"`
	NatLoopbackSupported   bool                  `json:"nat_loopback_supported"`
	SourceTitle            string                `json:"source_title"`
	ConnectionCount        int                   `json:"connection_count"`
	Connections            []TokenConnectionInfo `json:"connections,omitempty"`
}

type TokenInfo struct {
	Account   TokenAccountInfo    `json:"account"`
	Resources []TokenResourceInfo `json:"resources"`
}

type plexTVService interface {
	TokenDetails(ctx context.Context) (*components.UserPlexAccount, error)
	ServerResources(ctx context.Context) ([]components.PlexDevice, error)
}

type sdkPlexTVService struct {
	sdk       *plexgo.PlexAPI
	serverURL string
}

var newPlexTVService = func(token string) plexTVService {
	return newSDKPlexTVService(token, PlexTVURL)
}

func newSDKPlexTVService(token, serverURL string) plexTVService {
	return &sdkPlexTVService{
		sdk: plexgo.New(
			plexgo.WithSecurity(token),
			plexgo.WithAccepts(components.AcceptsApplicationJSON),
			plexgo.WithClientIdentifier(DefaultClientID),
			plexgo.WithProduct(DefaultProduct),
			plexgo.WithVersion(DefaultVersion),
			plexgo.WithPlatform(DefaultPlatform),
			plexgo.WithTimeout(DefaultTimeout),
		),
		serverURL: serverURL,
	}
}

func (s *sdkPlexTVService) TokenDetails(ctx context.Context) (*components.UserPlexAccount, error) {
	res, err := s.sdk.Authentication.GetTokenDetails(
		ctx,
		operations.GetTokenDetailsRequest{},
		operations.WithServerURL(s.serverURL),
	)
	if err != nil {
		return nil, err
	}
	if res == nil || res.UserPlexAccount == nil {
		return nil, fmt.Errorf("empty token details response")
	}
	return res.UserPlexAccount, nil
}

func (s *sdkPlexTVService) ServerResources(ctx context.Context) ([]components.PlexDevice, error) {
	res, err := s.sdk.Plex.GetServerResources(
		ctx,
		operations.GetServerResourcesRequest{
			IncludeHTTPS: operations.IncludeHTTPSTrue.ToPointer(),
			IncludeRelay: operations.IncludeRelayTrue.ToPointer(),
			IncludeIPv6:  operations.IncludeIPv6True.ToPointer(),
		},
		operations.WithServerURL(s.serverURL),
	)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, fmt.Errorf("empty server resources response")
	}
	return res.PlexDevices, nil
}

func getTokenDetails(ctx context.Context, token string) (*components.UserPlexAccount, error) {
	if token == "" {
		return nil, ErrNoCredentials
	}

	details, err := newPlexTVService(token).TokenDetails(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	return details, nil
}

func getServerResources(ctx context.Context, token string) ([]components.PlexDevice, error) {
	if token == "" {
		return nil, ErrNoCredentials
	}

	resources, err := newPlexTVService(token).ServerResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to discover servers: %v", ErrAuthFailed, err)
	}

	return resources, nil
}

func InspectToken(ctx context.Context, token string) (*TokenInfo, error) {
	account, err := getTokenDetails(ctx, token)
	if err != nil {
		return nil, err
	}

	resources, err := getServerResources(ctx, token)
	if err != nil {
		return nil, err
	}

	return &TokenInfo{
		Account:   normalizeTokenAccountInfo(account),
		Resources: normalizeTokenResourceInfos(resources),
	}, nil
}

func normalizeTokenAccountInfo(account *components.UserPlexAccount) TokenAccountInfo {
	if account == nil {
		return TokenAccountInfo{}
	}

	info := TokenAccountInfo{
		ID:               account.ID,
		Title:            account.Title,
		Username:         account.Username,
		Email:            account.Email,
		FriendlyName:     account.FriendlyName,
		UUID:             account.UUID,
		Home:             boolValue(account.Home),
		HomeAdmin:        boolValue(account.HomeAdmin),
		Guest:            boolValue(account.Guest),
		Restricted:       boolValue(account.Restricted),
		Confirmed:        boolValue(account.Confirmed),
		Anonymous:        boolValue(account.Anonymous),
		HasPassword:      boolValue(account.HasPassword),
		TwoFactorEnabled: boolValue(account.TwoFactorEnabled),
		Roles:            append([]string(nil), account.Roles...),
		Entitlements:     append([]string(nil), account.Entitlements...),
	}

	if account.Subscription != nil {
		info.SubscriptionActive = boolValue(account.Subscription.Active)
		info.SubscriptionStatus = stringValueEnum(account.Subscription.Status)
		info.SubscriptionPlan = stringValue(account.Subscription.Plan)
	}

	return info
}

func normalizeTokenResourceInfos(resources []components.PlexDevice) []TokenResourceInfo {
	items := make([]TokenResourceInfo, 0, len(resources))
	for _, resource := range resources {
		items = append(items, normalizeTokenResourceInfo(resource))
	}
	return items
}

func normalizeTokenResourceInfo(resource components.PlexDevice) TokenResourceInfo {
	connections := make([]TokenConnectionInfo, 0, len(resource.Connections))
	for _, conn := range resource.Connections {
		connections = append(connections, TokenConnectionInfo{
			Protocol: string(conn.Protocol),
			Address:  conn.Address,
			Port:     conn.Port,
			URI:      conn.URI,
			Local:    conn.Local,
			Relay:    conn.Relay,
			IPv6:     conn.IPv6,
		})
	}

	return TokenResourceInfo{
		Name:                   resource.Name,
		Product:                resource.Product,
		ProductVersion:         resource.ProductVersion,
		Platform:               stringValue(resource.Platform),
		PlatformVersion:        stringValue(resource.PlatformVersion),
		Device:                 stringValue(resource.Device),
		ClientIdentifier:       resource.ClientIdentifier,
		Provides:               resource.Provides,
		PublicAddress:          resource.PublicAddress,
		AccessToken:            resource.AccessToken,
		Owned:                  resource.Owned,
		Home:                   resource.Home,
		Synced:                 resource.Synced,
		Relay:                  resource.Relay,
		Presence:               resource.Presence,
		HTTPSRequired:          resource.HTTPSRequired,
		PublicAddressMatches:   resource.PublicAddressMatches,
		DNSRebindingProtection: resource.DNSRebindingProtection,
		NatLoopbackSupported:   resource.NatLoopbackSupported,
		SourceTitle:            stringValue(resource.SourceTitle),
		ConnectionCount:        len(connections),
		Connections:            connections,
	}
}

func normalizeServerResources(resources []components.PlexDevice) []ServerResource {
	servers := make([]ServerResource, 0, len(resources))
	for _, resource := range resources {
		if !strings.Contains(resource.Product, "Plex Media Server") {
			continue
		}

		connections := make([]ServerConnection, 0, len(resource.Connections))
		for _, conn := range resource.Connections {
			connections = append(connections, ServerConnection{
				Protocol: string(conn.Protocol),
				URI:      conn.URI,
				Local:    conn.Local,
				Relay:    conn.Relay,
			})
		}

		servers = append(servers, ServerResource{
			Name:             resource.Name,
			Product:          resource.Product,
			ClientIdentifier: resource.ClientIdentifier,
			Owned:            resource.Owned,
			Presence:         resource.Presence,
			AccessToken:      resource.AccessToken,
			Connections:      connections,
		})
	}

	return servers
}

func boolValue(v *bool) bool {
	return v != nil && *v
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

type stringer interface {
	~string
}

func stringValueEnum[T stringer](v *T) string {
	if v == nil {
		return ""
	}
	return string(*v)
}
