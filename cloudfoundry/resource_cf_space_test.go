package cloudfoundry

import (
	"fmt"
	"strconv"
	"testing"

	"code.cloudfoundry.org/cli/cf/errors"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
	"github.com/terraform-providers/terraform-provider-cf/cloudfoundry/cfapi"
)

const spaceResource = `

data "cf_quota" "default" {
    name = "default"
}
resource "cf_asg" "svc" {
	name = "app-services"
    rule {
        protocol = "all"
        destination = "192.168.100.0/24"
    }
}
resource "cf_user" "tl" {
	name = "teamlead@acme.com"
	password = "password"
}
resource "cf_user" "dev1" {
    name = "developer1@acme.com"
	password = "password"
}
resource "cf_user" "dev2" {
    name = "developer2@acme.com"
	password = "password"
}
resource "cf_user" "dev3" {
    name = "developer3@acme.com"
	password = "password"
}
resource "cf_user" "adr" {
    name = "auditor@acme.com"
	password = "password"
}
resource "cf_org" "org1" {
	name = "organization-one"
}
resource "cf_quota" "dev" {
	name = "50g"
	org = "${cf_org.org1.id}"
    allow_paid_service_plans = true
    instance_memory = 1024
    total_memory = 51200
    total_app_instances = 100
    total_routes = 100
    total_services = 150
}

resource "cf_space" "space1" {
	name = "space-one"
	org = "${cf_org.org1.id}"
	quota = "${cf_quota.dev.id}"
	asgs = [ "${cf_asg.svc.id}" ]
    managers = [ 
        "${cf_user.tl.id}" 
    ]
    developers = [ 
        "${cf_user.tl.id}",
        "${cf_user.dev1.id}",
		"${cf_user.dev2.id}" 
    ]
    auditors = [ 
        "${cf_user.adr.id}",
		"${cf_user.dev3.id}" 
    ]
	allow_ssh = true
}
`

const spaceResourceUpdate = `

data "cf_quota" "default" {
    name = "default"
}
resource "cf_asg" "svc" {
	name = "app-services"
    rule {
        protocol = "all"
        destination = "192.168.100.0/24"
    }
}
resource "cf_user" "tl" {
    name = "teamlead@acme.com"
	password = "password"
}
resource "cf_user" "dev1" {
    name = "developer1@acme.com"
	password = "password"
}
resource "cf_user" "dev2" {
    name = "developer2@acme.com"
	password = "password"
}
resource "cf_user" "dev3" {
    name = "developer3@acme.com"
	password = "password"
}
resource "cf_user" "adr" {
    name = "auditor@acme.com"
	password = "password"
}
resource "cf_org" "org1" {
	name = "organization-one"
}
resource "cf_quota" "dev" {
	name = "50g"
	org = "${cf_org.org1.id}"
    allow_paid_service_plans = true
    instance_memory = 1024
    total_memory = 51200
    total_app_instances = 100
    total_routes = 100
    total_services = 150
}

resource "cf_space" "space1" {
	name = "space-one-updated"
	org = "${cf_org.org1.id}"
	quota = "${cf_quota.dev.id}"
	asgs = [ "${cf_asg.svc.id}" ]
    managers = [ 
        "${cf_user.tl.id}" 
    ]
    developers = [ 
        "${cf_user.tl.id}",
        "${cf_user.dev1.id}",
    ]
    auditors = [ 
        "${cf_user.adr.id}",
		"${cf_user.dev2.id}" 
    ]
	allow_ssh = true
}
`

func TestAccSpace_normal(t *testing.T) {

	ref := "cf_space.space1"
	refUserRemoved := "cf_user.dev3"

	resource.Test(t,
		resource.TestCase{
			PreCheck:     func() { testAccPreCheck(t) },
			Providers:    testAccProviders,
			CheckDestroy: testAccCheckSpaceDestroyed("space-one"),
			Steps: []resource.TestStep{

				resource.TestStep{
					Config: spaceResource,
					Check: resource.ComposeTestCheckFunc(
						testAccCheckSpaceExists(ref, nil),
						resource.TestCheckResourceAttr(
							ref, "name", "space-one"),
						resource.TestCheckResourceAttr(
							ref, "asgs.#", "1"),
						resource.TestCheckResourceAttr(
							ref, "managers.#", "1"),
						resource.TestCheckResourceAttr(
							ref, "developers.#", "3"),
						resource.TestCheckResourceAttr(
							ref, "auditors.#", "2"),
					),
				},

				resource.TestStep{
					Config: spaceResourceUpdate,
					Check: resource.ComposeTestCheckFunc(
						testAccCheckSpaceExists(ref, &refUserRemoved),
						resource.TestCheckResourceAttr(
							ref, "name", "space-one-updated"),
						resource.TestCheckResourceAttr(
							ref, "asgs.#", "1"),
						resource.TestCheckResourceAttr(
							ref, "managers.#", "1"),
						resource.TestCheckResourceAttr(
							ref, "developers.#", "2"),
						resource.TestCheckResourceAttr(
							ref, "auditors.#", "2"),
					),
				},
			},
		})
}

func testAccCheckSpaceExists(resource string, refUserRemoved *string) resource.TestCheckFunc {

	return func(s *terraform.State) (err error) {

		session := testAccProvider.Meta().(*cfapi.Session)

		rs, ok := s.RootModule().Resources[resource]
		if !ok {
			return fmt.Errorf("quota '%s' not found in terraform state", resource)
		}

		session.Log.DebugMessage(
			"terraform state for resource '%s': %# v",
			resource, rs)

		id := rs.Primary.ID
		attributes := rs.Primary.Attributes

		var (
			space cfapi.CCSpace

			runningAsgs                    []string
			spaceAsgs, asgs                []interface{}
			managers, developers, auditors []interface{}
		)

		sm := session.SpaceManager()
		if space, err = sm.ReadSpace(id); err != nil {
			return
		}
		session.Log.DebugMessage(
			"retrieved space for resource '%s' with id '%s': %# v",
			resource, id, space)

		if err := assertEquals(attributes, "name", space.Name); err != nil {
			return err
		}
		if err := assertEquals(attributes, "org", space.OrgGUID); err != nil {
			return err
		}
		if err := assertEquals(attributes, "quota", space.QuotaGUID); err != nil {
			return err
		}
		if err := assertEquals(attributes, "allow_ssh", strconv.FormatBool(space.AllowSSH)); err != nil {
			return err
		}

		if runningAsgs, err = session.ASGManager().Running(); err != nil {
			return err
		}
		if spaceAsgs, err = sm.ListASGs(id); err != nil {
			return
		}
		for _, a := range spaceAsgs {
			if !isStringInList(runningAsgs, a.(string)) {
				asgs = append(asgs, a)
			}
		}
		session.Log.DebugMessage(
			"retrieved asgs of space identified resource '%s': %# v",
			resource, asgs)

		if err := assertSetEquals(attributes, "asgs", asgs); err != nil {
			return err
		}

		if managers, err = sm.ListUsers(id, cfapi.SpaceRoleManager); err != nil {
			return
		}
		session.Log.DebugMessage(
			"retrieved managers of space identified resource '%s': %# v",
			resource, managers)

		if err := assertSetEquals(attributes, "managers", managers); err != nil {
			return err
		}

		if developers, err = sm.ListUsers(id, cfapi.SpaceRoleDeveloper); err != nil {
			return
		}
		session.Log.DebugMessage(
			"retrieved developers of space identified resource '%s': %# v",
			resource, developers)

		if err := assertSetEquals(attributes, "developers", developers); err != nil {
			return err
		}

		if auditors, err = sm.ListUsers(id, cfapi.SpaceRoleAuditor); err != nil {
			return
		}
		session.Log.DebugMessage(
			"retrieved managers of space identified resource '%s': %# v",
			resource, auditors)

		if err := assertSetEquals(attributes, "auditors", auditors); err != nil {
			return err
		}

		err = testUserRemovedFromOrg(refUserRemoved, space.OrgGUID, session.OrgManager(), s)

		return
	}
}

func testAccCheckSpaceDestroyed(spacename string) resource.TestCheckFunc {

	return func(s *terraform.State) error {

		session := testAccProvider.Meta().(*cfapi.Session)
		if _, err := session.SpaceManager().FindSpace(spacename); err != nil {
			switch err.(type) {
			case *errors.ModelNotFoundError:
				return nil
			default:
				return err
			}
		}
		return fmt.Errorf("space with name '%s' still exists in cloud foundry", spacename)
	}
}
