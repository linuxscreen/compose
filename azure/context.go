/*
   Copyright 2020 Docker, Inc.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package azure

import (
	"context"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2018-05-01/resources"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/tj/survey/terminal"

	"github.com/docker/api/context/store"
)

type contextCreateACIHelper struct {
	selector            userSelector
	resourceGroupHelper ACIResourceGroupHelper
}

func newContextCreateHelper() contextCreateACIHelper {
	return contextCreateACIHelper{
		selector:            cliUserSelector{},
		resourceGroupHelper: aciResourceGroupHelperImpl{},
	}
}

func (helper contextCreateACIHelper) createContextData(ctx context.Context, opts map[string]string) (interface{}, string, error) {
	var subscriptionID string
	if opts["aciSubscriptionID"] != "" {
		subscriptionID = opts["aciSubscriptionID"]
	} else {
		subs, err := helper.resourceGroupHelper.GetSubscriptionIDs(ctx)
		if err != nil {
			return nil, "", err
		}
		subscriptionID, err = helper.chooseSub(subs)
		if err != nil {
			return nil, "", err
		}
	}

	var group resources.Group
	var err error

	if opts["aciResourceGroup"] != "" {
		group, err = helper.resourceGroupHelper.GetGroup(ctx, subscriptionID, opts["aciResourceGroup"])
		if err != nil {
			return nil, "", errors.Wrapf(err, "Could not find resource group %q", opts["aciResourceGroup"])
		}
	} else {
		groups, err := helper.resourceGroupHelper.ListGroups(ctx, subscriptionID)
		if err != nil {
			return nil, "", err
		}
		group, err = helper.chooseGroup(ctx, subscriptionID, opts, groups)
		if err != nil {
			return nil, "", err
		}
	}

	location := opts["aciLocation"]
	if location == "" {
		location = *group.Location
	}

	description := fmt.Sprintf("%s@%s", *group.Name, location)
	if opts["description"] != "" {
		description = fmt.Sprintf("%s (%s)", opts["description"], description)
	}

	return store.AciContext{
		SubscriptionID: subscriptionID,
		Location:       location,
		ResourceGroup:  *group.Name,
	}, description, nil
}

func (helper contextCreateACIHelper) createGroup(ctx context.Context, subscriptionID, location string) (resources.Group, error) {
	if location == "" {
		location = "eastus"
	}
	gid := uuid.New().String()
	g, err := helper.resourceGroupHelper.CreateOrUpdate(ctx, subscriptionID, gid, resources.Group{
		Location: &location,
	})
	if err != nil {
		return resources.Group{}, err
	}

	fmt.Printf("Resource group %q (%s) created\n", *g.Name, *g.Location)

	return g, nil
}

func (helper contextCreateACIHelper) chooseGroup(ctx context.Context, subscriptionID string, opts map[string]string, groups []resources.Group) (resources.Group, error) {
	groupNames := []string{"create a new resource group"}
	for _, g := range groups {
		groupNames = append(groupNames, fmt.Sprintf("%s (%s)", *g.Name, *g.Location))
	}

	group, err := helper.selector.userSelect("Select a resource group", groupNames)
	if err != nil {
		if err == terminal.InterruptErr {
			os.Exit(0)
		}

		return resources.Group{}, err
	}

	if group == 0 {
		return helper.createGroup(ctx, subscriptionID, opts["aciLocation"])
	}

	return groups[group-1], nil
}

func (helper contextCreateACIHelper) chooseSub(subs []subscription.Model) (string, error) {
	if len(subs) == 1 {
		sub := subs[0]
		fmt.Println("Using only available subscription : " + *sub.DisplayName + "(" + *sub.SubscriptionID + ")")
		return *sub.SubscriptionID, nil
	}
	var options []string
	for _, sub := range subs {
		options = append(options, *sub.DisplayName+"("+*sub.SubscriptionID+")")
	}
	selected, err := helper.selector.userSelect("Select a subscription ID", options)
	if err != nil {
		if err == terminal.InterruptErr {
			os.Exit(0)
		}
		return "", err
	}
	return *subs[selected].SubscriptionID, nil
}

type userSelector interface {
	userSelect(message string, options []string) (int, error)
}

type cliUserSelector struct{}

func (us cliUserSelector) userSelect(message string, options []string) (int, error) {
	qs := &survey.Select{
		Message: message,
		Options: options,
	}
	var selected int
	err := survey.AskOne(qs, &selected, nil)
	return selected, err
}
