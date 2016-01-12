package main

import (
	"fmt"
	"strings"
)

const fmtEntitlement = `
{
  "returnPayloadOnError" : true,
  "operations" : [ {
    "method" : "POST",
    "data" : {
      "catalogItemId" : "%s",
      "subjectType" : "%s",
      "subjectId" : "%s",
      "activationPolicy" : "AUTOMATIC"
    }
  } ]
}`

// Create entitlement for the given user or group
func maybeEntitle(ctx *httpContext, itemID, subjName, subjType, nameAttr, appName string) {
	if subjName != "" {
		subjID, err := scimGetID(ctx, strings.Title(subjType + "s"), nameAttr, subjName)
		if err == nil {
			err = entitleSubject(ctx, subjID, strings.ToUpper(subjType + "s"), itemID)
		}
		if err != nil {
			ctx.log.err("Could not entitle %s \"%s\" to app \"%s\", error: %v\n", subjType, subjName, appName, err)
		} else {
			ctx.log.info("Entitled %s \"%s\" to app \"%s\".\n", subjType, subjName, appName)
		}
	}
}

func entitleSubject(ctx *httpContext, subjectId, subjectType, itemID string) error {
	inp := fmt.Sprintf(fmtEntitlement, itemID, subjectType, subjectId)
	ctx.accept("bulk.sync.response").contentType("entitlements.definition.bulk")
	return ctx.request("POST", "entitlements/definitions", inp, nil)
}

// Get entitlement for the given user whose username is 'name'
// rtypeName has been validated before and is one of 'user', 'group' or 'app'
func getEntitlement(ctx *httpContext, rtypeName, name string) {
	var resType, id string
	body := make(map[string]interface{})
	switch rtypeName {
	case "user":
		resType, id = "users", scimNameToID(ctx, "Users", "userName", name)
	case "group":
		resType, id = "groups", scimNameToID(ctx, "Groups", "displayName", name)
	case "app":
		resType, id = "catalogitems", name
	}
	if id == "" {
		return
	}
	path := fmt.Sprintf("entitlements/definitions/%s/%s", resType, id)
	if err := ctx.request("GET", path, nil, &body); err != nil {
		ctx.log.err("Error: %v\n", err)
	} else {
		ctx.log.ppf("Entitlements", body["items"], []string{"Entitlements",
			"catalogItemId", "subjectType", "subjectId", "activationPolicy"})
	}
}
