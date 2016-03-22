package main

import (
	"fmt"
	"net/url"
	"strconv"
)

const coreSchemaURN = "urn:scim:schemas:core:1.0"

type dispValue struct {
	Display, Value string `json:",omitempty"`
}

type nameAttr struct {
	GivenName, FamilyName string `json:",omitempty"`
}

type userAccount struct {
	Schemas               []string                                                   `json:",omitempty"`
	UserName              string                                                     `json:",omitempty"`
	Id                    string                                                     `json:",omitempty"`
	Active                bool                                                       `json:",omitempty"`
	Emails, Groups, Roles []dispValue                                                `json:",omitempty"`
	Meta                  *struct{ Created, LastModified, Location, Version string } `json:",omitempty"`
	Name                  *nameAttr                                                  `json:",omitempty"`
	WksExt                *struct{ InternalUserType, UserStatus string }             `json:"urn:scim:schemas:extension:workspace:1.0,omitempty"`
	Password              string                                                     `json:",omitempty"`
}

type memberValue struct {
	Value, Type, Operation string `json:",omitempty"`
}

type memberPatch struct {
	Schemas []string      `json:",omitempty"`
	Members []memberValue `json:",omitempty"`
}

type basicUser struct {
	Name, Given, Family, Email, Pwd string `yaml:",omitempty,flow"`
}

func scimGetByName(ctx *httpContext, resType, nameAttr, name string) (item map[string]interface{}, err error) {
	output := &struct {
		Resources                              []map[string]interface{}
		ItemsPerPage, TotalResults, StartIndex uint
		Schemas                                []string
	}{}
	vals := url.Values{"count": {"10000"}, "filter": {fmt.Sprintf("%s eq \"%s\"", nameAttr, name)}}
	path := fmt.Sprintf("scim/%v?%v", resType, vals.Encode())
	if err = ctx.request("GET", path, nil, &output); err != nil {
		return
	}
	for _, v := range output.Resources {
		if caselessEqual(name, v[nameAttr]) {
			if item != nil {
				return nil, fmt.Errorf("multiple %v found named \"%s\"", resType, name)
			} else {
				item = v
			}
		}
	}
	if item == nil {
		err = fmt.Errorf("no %v found named \"%s\"", resType, name)
	}
	return
}

func scimGetID(ctx *httpContext, resType, nameAttr, name string) (string, error) {
	if item, err := scimGetByName(ctx, resType, nameAttr, name); err != nil {
		return "", err
	} else if id, ok := item["id"].(string); !ok {
		return "", fmt.Errorf("no id returned for \"%s\"", name)
	} else {
		return id, nil
	}
}
// @param count the number of records to return
// @param summaryLabels keys to filter the results of what to display
func scimList(ctx *httpContext, count int, filter string, resType string, summaryLabels ...string) {
	vals := url.Values{}
	if count > 0 {
		vals.Set("count", strconv.Itoa(count))
	}
	if filter != "" {
		vals.Set("filter", filter)
	}
	path := fmt.Sprintf("scim/%s?%v", resType, vals.Encode())
	outp := make(map[string]interface{})
	if err := ctx.request("GET", path, nil, &outp); err != nil {
		ctx.log.err("Error getting SCIM resources of type %s: %v\n", resType, err)
	} else {
		ctx.log.ppf(resType, outp["Resources"], summaryLabels)
	}
}

func scimPatch(ctx *httpContext, resType, id string, input interface{}) error {
	ctx.header("X-HTTP-Method-Override", "PATCH")
	path := fmt.Sprintf("scim/%s/%s", resType, id)
	return ctx.request("POST", path, input, nil)
}

func scimNameToID(ctx *httpContext, resType, nameAttr, name string) string {
	if id, err := scimGetID(ctx, resType, nameAttr, name); err == nil {
		return id
	} else {
		ctx.log.err("Error getting SCIM %s ID of %s: %v\n", resType, name, err)
	}
	return ""
}

func scimMember(ctx *httpContext, resType, nameAttr, rname, uname string, remove bool) {
	rid, uid := scimNameToID(ctx, resType, nameAttr, rname), scimNameToID(ctx, "Users", "userName", uname)
	if rid == "" || uid == "" {
		return
	}
	patch := memberPatch{Schemas: []string{coreSchemaURN}, Members: []memberValue{{Value: uid, Type: "User"}}}
	if remove {
		patch.Members[0].Operation = "delete"
	}
	if err := scimPatch(ctx, resType, rid, &patch); err != nil {
		ctx.log.err("Error updating SCIM resource %s of type %s: %v\n", rname, resType, err)
	} else {
		ctx.log.info("Updated SCIM resource %s of type %s\n", rname, resType)
	}
}

func scimGet(ctx *httpContext, resType, nameAttr, rname string) {
	if item, err := scimGetByName(ctx, resType, nameAttr, rname); err != nil {
		ctx.log.err("Error getting SCIM resource named %s of type %s: %v\n", rname, resType, err)
	} else {
		ctx.log.pp("", item)
	}
}

func addUser(ctx *httpContext, u *basicUser) error {
	acct := &userAccount{UserName: u.Name, Schemas: []string{coreSchemaURN}}
	acct.Password = u.Pwd
	acct.Name = &nameAttr{FamilyName: stringOrDefault(u.Family, u.Name), GivenName: stringOrDefault(u.Given, u.Name)}
	acct.Emails = []dispValue{{Value: stringOrDefault(u.Email, u.Name+"@example.com")}}
	ctx.log.pp("add user: ", acct)
	return ctx.request("POST", "scim/Users", acct, acct)
}

func cmdLoadUsers(ctx *httpContext, fileName string) {
	var newUsers []basicUser
	if err := getYamlFile(fileName, &newUsers); err != nil {
		ctx.log.err("could not read file of bulk users: %v\n", err)
	} else {
		for k, v := range newUsers {
			if err := addUser(ctx, &v); err != nil {
				ctx.log.err("Error adding user, line %d, name %s: %v\n", k+1, v.Name, err)
			} else {
				ctx.log.info("added user %s\n", v.Name)
			}
		}
	}
}

func cmdAddUser(ctx *httpContext, user *basicUser) {
	if err := addUser(ctx, user); err != nil {
		ctx.log.err("Error creating user: %v\n", err)
	} else {
		ctx.log.info("User successfully added\n")
	}
}

func cmdUpdateUser(ctx *httpContext, user *basicUser) {
	if id := scimNameToID(ctx, "Users", "userName", user.Name); id != "" {
		acct := userAccount{Schemas: []string{coreSchemaURN}}
		if user.Given != "" || user.Family != "" {
			acct.Name = &nameAttr{FamilyName: user.Family, GivenName: user.Given}
		}
		if user.Email != "" {
			acct.Emails = []dispValue{{Value: user.Email}}
		}
		if err := scimPatch(ctx, "Users", id, &acct); err != nil {
			ctx.log.err("Error updating user \"%s\": %v\n", user.Name, err)
		} else {
			ctx.log.info("User \"%s\" updated\n", user.Name)
		}
	}
}

func scimDelete(ctx *httpContext, resType, nameAttr, rname string) {
	if id := scimNameToID(ctx, resType, nameAttr, rname); id != "" {
		path := fmt.Sprintf("scim/%s/%s", resType, id)
		if err := ctx.request("DELETE", path, nil, nil); err != nil {
			ctx.log.err("Error deleting %s %s: %v\n", resType, rname, err)
		} else {
			ctx.log.info("%s \"%s\" deleted\n", resType, rname)
		}
	}
}

func cmdSetPassword(ctx *httpContext, name, pwd string) {
	if id := scimNameToID(ctx, "Users", "userName", name); id != "" {
		acct := userAccount{Schemas: []string{coreSchemaURN}, Password: pwd}
		if err := scimPatch(ctx, "Users", id, &acct); err != nil {
			ctx.log.err("Error updating user %s: %v\n", name, err)
		} else {
			ctx.log.info("User \"%s\" updated\n", name)
		}
	}
}
