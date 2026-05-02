package auth

import (
	"crypto/tls"
	"fmt"
	"log"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

type LDAPConfig struct {
	Host         string
	Port         int
	UseTLS       bool
	StartTLS     bool
	SkipVerify   bool
	BindDN       string
	BindPassword string
	BaseDN       string
	UserFilter   string
	GroupFilter  string
	GroupBaseDN  string
}

func (c *LDAPConfig) IsConfigured() bool {
	return c.Host != "" && c.BaseDN != ""
}

func (c *LDAPConfig) serverURL() string {
	scheme := "ldap"
	if c.UseTLS {
		scheme = "ldaps"
	}
	port := c.Port
	if port == 0 {
		if c.UseTLS {
			port = 636
		} else {
			port = 389
		}
	}
	return fmt.Sprintf("%s://%s:%d", scheme, c.Host, port)
}

func ldapAuthenticate(config *LDAPConfig, username, password string) (bool, bool, error) {
	if !config.IsConfigured() {
		return false, false, fmt.Errorf("LDAP not configured")
	}

	conn, err := ldap.DialURL(config.serverURL())
	if err != nil {
		return false, false, fmt.Errorf("LDAP connection failed: %w", err)
	}
	defer conn.Close()

	if config.StartTLS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: config.SkipVerify,
			ServerName:         config.Host,
		}
		if err := conn.StartTLS(tlsConfig); err != nil {
			return false, false, fmt.Errorf("LDAP StartTLS failed: %w", err)
		}
	}

	if config.BindDN != "" {
		if err := conn.Bind(config.BindDN, config.BindPassword); err != nil {
			return false, false, fmt.Errorf("LDAP service bind failed: %w", err)
		}
	}

	userFilter := config.UserFilter
	if userFilter == "" {
		userFilter = "(sAMAccountName=%s)"
	}
	filter := fmt.Sprintf(userFilter, ldap.EscapeFilter(username))

	searchRequest := ldap.NewSearchRequest(
		config.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 1, 0, false,
		filter,
		[]string{"dn", "memberOf"},
		nil,
	)

	result, err := conn.Search(searchRequest)
	if err != nil {
		return false, false, fmt.Errorf("LDAP user search failed: %w", err)
	}

	if len(result.Entries) == 0 {
		return false, false, nil
	}

	userDN := result.Entries[0].DN

	if err := conn.Bind(userDN, password); err != nil {
		log.Printf("LDAP: Authentication failed for user %s", username)
		return false, false, nil
	}

	log.Printf("LDAP: User %s authenticated successfully (DN: %s)", username, userDN)

	isAdmin := false
	if config.GroupFilter != "" {
		memberOf := result.Entries[0].GetAttributeValues("memberOf")
		groupFilter := strings.ToLower(config.GroupFilter)
		for _, group := range memberOf {
			if strings.Contains(strings.ToLower(group), groupFilter) {
				isAdmin = true
				break
			}
		}

		if !isAdmin {
			groupBaseDN := config.GroupBaseDN
			if groupBaseDN == "" {
				groupBaseDN = config.BaseDN
			}

			if config.BindDN != "" {
				conn.Bind(config.BindDN, config.BindPassword)
			}

			groupSearchFilter := fmt.Sprintf("(&(%s)(member=%s))", config.GroupFilter, ldap.EscapeFilter(userDN))
			groupSearch := ldap.NewSearchRequest(
				groupBaseDN,
				ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 1, 0, false,
				groupSearchFilter,
				[]string{"dn"},
				nil,
			)

			groupResult, err := conn.Search(groupSearch)
			if err == nil && len(groupResult.Entries) > 0 {
				isAdmin = true
			}
		}

		if isAdmin {
			log.Printf("LDAP: User %s is a member of admin group", username)
		}
	} else {
		isAdmin = true
	}

	return true, isAdmin, nil
}
