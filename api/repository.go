/*
   Copyright (c) 2016 VMware, Inc. All Rights Reserved.
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

package api

import (
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/distribution/manifest/schema1"
	"github.com/vmware/harbor/dao"
	"github.com/vmware/harbor/models"
	"github.com/vmware/harbor/service/cache"
	svc_utils "github.com/vmware/harbor/service/utils"
	"github.com/vmware/harbor/utils/log"
	"github.com/vmware/harbor/utils/registry"

	registry_error "github.com/vmware/harbor/utils/registry/error"

	"github.com/vmware/harbor/utils/registry/auth"
)

// RepositoryAPI handles request to /api/repositories /api/repositories/tags /api/repositories/manifests, the parm has to be put
// in the query string as the web framework can not parse the URL if it contains veriadic sectors.
type RepositoryAPI struct {
	BaseAPI
}

type repoPaging struct {
	Pages    int `json:"pages"`
	RepoList []string `json:"repoList"`
}

// Get ...
func (ra *RepositoryAPI) Get() {
	projectID, err := ra.GetInt64("project_id")
	if err != nil {
		log.Errorf("Failed to get project id, error: %v", err)
		ra.RenderError(http.StatusBadRequest, "Invalid project id")
		return
	}
	p, err := dao.GetProjectByID(projectID)
	if err != nil {
		log.Errorf("Error occurred in GetProjectById, error: %v", err)
		ra.CustomAbort(http.StatusInternalServerError, "Internal error.")
	}
	if p == nil {
		log.Warningf("Project with Id: %d does not exist", projectID)
		ra.RenderError(http.StatusNotFound, "")
		return
	}

	if p.Public == 0 {
		var userID int

		if svc_utils.VerifySecret(ra.Ctx.Request) {
			userID = 1
		} else {
			userID = ra.ValidateUser()
		}

		if !checkProjectPermission(userID, projectID) {
			ra.RenderError(http.StatusForbidden, "")
			return
		}
	}

	repoList, err := cache.GetRepoFromCache()
	if err != nil {
		log.Errorf("Failed to get repo from cache, error: %v", err)
		ra.RenderError(http.StatusInternalServerError, "internal sever error")
	}

	projectName := p.Name
	q := ra.GetString("q")
	var resp []string
	if len(q) > 0 {
		for _, r := range repoList {
			if strings.Contains(r, "/") && strings.Contains(r[strings.LastIndex(r, "/")+1:], q) && r[0:strings.LastIndex(r, "/")] == projectName {
				resp = append(resp, r)
			}
			labelList,_ := dao.GetRepoLabels(r)
			for _, label := range labelList{
				if strings.Contains(label, q) {
					resp = append(resp, label)
				}
			}
		}
		ra.Data["json"] = resp
	} else if len(projectName) > 0 {
		for _, r := range repoList {
			if strings.Contains(r, "/") && r[0:strings.LastIndex(r, "/")] == projectName {
				resp = append(resp, r)
			}
		}
		ra.Data["json"] = resp
	} else {
		ra.Data["json"] = repoList
	}
	ra.ServeJSON()
}

func getSubPage(strs []string, pageNum int) (repoPaging) {
	length := len(strs)
	var repoPage repoPaging

	switch {
	case length >= (pageNum+1)*5:
		repoPage.RepoList = strs[pageNum*5 : (pageNum+1)*5]

	case length < (pageNum+1)*5 && length >= (pageNum)*5:
		repoPage.RepoList = strs[pageNum*5 : length]
	}

	if length % 5==0{
		repoPage.Pages = length/5
	}else{
		repoPage.Pages = length/5+1
	}
	log.Info("repoPages: ", repoPage.Pages)
	log.Info("repoList: ", repoPage.RepoList)

	return repoPage
}

func (ra *RepositoryAPI) AddLabel() {
	repoName := ra.GetString("repo_name")
	if len(repoName) == 0 {
		ra.CustomAbort(http.StatusBadRequest, "repo_name is nil")
	}

	label := ra.GetString("label")
	if len(label) == 0 {
		ra.CustomAbort(http.StatusBadRequest, "label is nil")
	}

	repoLabel := models.RepoLabel{RepoName: repoName, Label: label}
	insertId, err := dao.AddLabel(repoLabel)
	if err != nil {
		ra.CustomAbort(http.StatusInternalServerError, "Failed to insert label to db")
	} else {
		ra.Data["json"] = insertId
	}
	ra.ServeJSON()
}

func (ra *RepositoryAPI) DeleteLabel() {
	repoName := ra.GetString("repo_name")
	if len(repoName) == 0 {
		ra.CustomAbort(http.StatusBadRequest, "repo_name is nil")
	}

	label := ra.GetString("label")
	if len(label) == 0 {
		ra.CustomAbort(http.StatusBadRequest, "label is nil")
	}

	repoLabel := models.RepoLabel{RepoName: repoName, Label: label}
	affectedRows, err := dao.DeletelLabel(repoLabel)

	if err != nil {
		ra.CustomAbort(http.StatusInternalServerError, "Failed to delete labels from db")
	} else {
		ra.Data["json"] = affectedRows
	}
	ra.ServeJSON()
}

func (ra *RepositoryAPI) GetLabels() {
	repoName := ra.GetString("repo_name")
	if len(repoName) == 0 {
		ra.CustomAbort(http.StatusBadRequest, "repo_name is nil")
	}

	labels, err := dao.GetRepoLabels(repoName)
	if err != nil {
		ra.CustomAbort(http.StatusInternalServerError, "Failed to get labels from db")
	} else {
		ra.Data["json"] = labels
	}
	ra.ServeJSON()
}

func (ra *RepositoryAPI) GetRepoNames() {
	label := ra.GetString("label")
	if len(label) == 0 {
		ra.CustomAbort(http.StatusBadRequest, "label is nil")
	}

	repos, err := dao.GetRepoNames(label)
	if err != nil {
		ra.CustomAbort(http.StatusInternalServerError, "Failed to get repo names from db")
	} else {
		ra.Data["json"] = repos
	}
}

// Delete ...
func (ra *RepositoryAPI) Delete() {
	repoName := ra.GetString("repo_name")
	if len(repoName) == 0 {
		ra.CustomAbort(http.StatusBadRequest, "repo_name is nil")
	}

	projectName := getProjectName(repoName)
	project, err := dao.GetProjectByName(projectName)
	if err != nil {
		log.Errorf("failed to get project %s: %v", projectName, err)
		ra.CustomAbort(http.StatusInternalServerError, "")
	}

	if project.Public == 0 {
		userID := ra.ValidateUser()
		if !hasProjectAdminRole(userID, project.ProjectID) {
			ra.CustomAbort(http.StatusForbidden, "")
		}
	}

	rc, err := ra.initRepositoryClient(repoName)
	if err != nil {
		log.Errorf("error occurred while initializing repository client for %s: %v", repoName, err)
		ra.CustomAbort(http.StatusInternalServerError, "internal error")
	}

	tags := []string{}
	tag := ra.GetString("tag")
	if len(tag) == 0 {
		tagList, err := rc.ListTag()
		if err != nil {
			if regErr, ok := err.(*registry_error.Error); ok {
				ra.CustomAbort(regErr.StatusCode, regErr.Detail)
			}

			log.Errorf("error occurred while listing tags of %s: %v", repoName, err)
			ra.CustomAbort(http.StatusInternalServerError, "internal error")
		}

		// TODO remove the logic if the bug of registry is fixed
		if len(tagList) == 0 {
			ra.CustomAbort(http.StatusNotFound, http.StatusText(http.StatusNotFound))
		}

		tags = append(tags, tagList...)
	} else {
		tags = append(tags, tag)
	}

	user, _, ok := ra.Ctx.Request.BasicAuth()
	if !ok {
		user, err = ra.getUsername()
		if err != nil {
			log.Errorf("failed to get user: %v", err)
		}
	}

	for _, t := range tags {
		if err := rc.DeleteTag(t); err != nil {
			if regErr, ok := err.(*registry_error.Error); ok {
				ra.CustomAbort(regErr.StatusCode, regErr.Detail)
			}

			log.Errorf("error occurred while deleting tags of %s: %v", repoName, err)
			ra.CustomAbort(http.StatusInternalServerError, "internal error")
		}
		log.Infof("delete tag: %s %s", repoName, t)
		go TriggerReplicationByRepository(repoName, []string{t}, models.RepOpDelete)

		go func(tag string) {
			if err := dao.AccessLog(user, projectName, repoName, tag, "delete"); err != nil {
				log.Errorf("failed to add access log: %v", err)
			}
		}(t)
	}

	go func() {
		log.Debug("refreshing catalog cache")
		if err := cache.RefreshCatalogCache(); err != nil {
			log.Errorf("error occurred while refresh catalog cache: %v", err)
		}
	}()
}

type tag struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// GetTags handles GET /api/repositories/tags
func (ra *RepositoryAPI) GetTags() {
	repoName := ra.GetString("repo_name")
	if len(repoName) == 0 {
		ra.CustomAbort(http.StatusBadRequest, "repo_name is nil")
	}

	projectName := getProjectName(repoName)
	project, err := dao.GetProjectByName(projectName)
	if err != nil {
		log.Errorf("failed to get project %s: %v", projectName, err)
		ra.CustomAbort(http.StatusInternalServerError, "")
	}

	if project.Public == 0 {
		userID := ra.ValidateUser()
		if !checkProjectPermission(userID, project.ProjectID) {
			ra.CustomAbort(http.StatusForbidden, "")
		}
	}

	rc, err := ra.initRepositoryClient(repoName)
	if err != nil {
		log.Errorf("error occurred while initializing repository client for %s: %v", repoName, err)
		ra.CustomAbort(http.StatusInternalServerError, "internal error")
	}

	tags := []string{}

	ts, err := rc.ListTag()
	if err != nil {
		if regErr, ok := err.(*registry_error.Error); ok {
			ra.CustomAbort(regErr.StatusCode, regErr.Detail)
		}

		log.Errorf("error occurred while listing tags of %s: %v", repoName, err)
		ra.CustomAbort(http.StatusInternalServerError, "internal error")
	}

	tags = append(tags, ts...)

	sort.Strings(tags)

	ra.Data["json"] = tags
	ra.ServeJSON()
}

// GetManifests handles GET /api/repositories/manifests
func (ra *RepositoryAPI) GetManifests() {
	repoName := ra.GetString("repo_name")
	tag := ra.GetString("tag")

	if len(repoName) == 0 || len(tag) == 0 {
		ra.CustomAbort(http.StatusBadRequest, "repo_name or tag is nil")
	}

	projectName := getProjectName(repoName)
	project, err := dao.GetProjectByName(projectName)
	if err != nil {
		log.Errorf("failed to get project %s: %v", projectName, err)
		ra.CustomAbort(http.StatusInternalServerError, "")
	}

	if project.Public == 0 {
		userID := ra.ValidateUser()
		if !checkProjectPermission(userID, project.ProjectID) {
			ra.CustomAbort(http.StatusForbidden, "")
		}
	}

	rc, err := ra.initRepositoryClient(repoName)
	if err != nil {
		log.Errorf("error occurred while initializing repository client for %s: %v", repoName, err)
		ra.CustomAbort(http.StatusInternalServerError, "internal error")
	}

	item := models.RepoItem{}

	mediaTypes := []string{schema1.MediaTypeManifest}
	_, _, payload, err := rc.PullManifest(tag, mediaTypes)
	if err != nil {
		if regErr, ok := err.(*registry_error.Error); ok {
			ra.CustomAbort(regErr.StatusCode, regErr.Detail)
		}

		log.Errorf("error occurred while getting manifest of %s:%s: %v", repoName, tag, err)
		ra.CustomAbort(http.StatusInternalServerError, "internal error")
	}
	mani := models.Manifest{}
	err = json.Unmarshal(payload, &mani)
	if err != nil {
		log.Errorf("Failed to decode json from response for manifests, repo name: %s, tag: %s, error: %v", repoName, tag, err)
		ra.RenderError(http.StatusInternalServerError, "Internal Server Error")
		return
	}
	v1Compatibility := mani.History[0].V1Compatibility

	err = json.Unmarshal([]byte(v1Compatibility), &item)
	if err != nil {
		log.Errorf("Failed to decode V1 field for repo, repo name: %s, tag: %s, error: %v", repoName, tag, err)
		ra.RenderError(http.StatusInternalServerError, "Internal Server Error")
		return
	}
	item.DurationDays = strconv.Itoa(int(time.Since(item.Created).Hours()/24)) + " days"

	ra.Data["json"] = item
	ra.ServeJSON()
}

func (ra *RepositoryAPI) initRepositoryClient(repoName string) (r *registry.Repository, err error) {
	endpoint := os.Getenv("REGISTRY_URL")

	username, password, ok := ra.Ctx.Request.BasicAuth()
	if ok {
		return newRepositoryClient(endpoint, getIsInsecure(), username, password,
			repoName, "repository", repoName, "pull", "push", "*")
	}

	username, err = ra.getUsername()
	if err != nil {
		return nil, err
	}

	return cache.NewRepositoryClient(endpoint, getIsInsecure(), username, repoName,
		"repository", repoName, "pull", "push", "*")
}

func (ra *RepositoryAPI) getUsername() (string, error) {
	// get username from session
	sessionUsername := ra.GetSession("username")
	if sessionUsername != nil {
		username, ok := sessionUsername.(string)
		if ok {
			return username, nil
		}
	}

	// if username does not exist in session, try to get userId from sessiion
	// and then get username from DB according to the userId
	sessionUserID := ra.GetSession("userId")
	if sessionUserID != nil {
		userID, ok := sessionUserID.(int)
		if ok {
			u := models.User{
				UserID: userID,
			}
			user, err := dao.GetUser(u)
			if err != nil {
				return "", err
			}

			return user.Username, nil
		}
	}

	return "", nil
}

//GetTopRepos handles request GET /api/repositories/top
func (ra *RepositoryAPI) GetTopRepos() {
	var err error
	var countNum int
	count := ra.GetString("count")
	if len(count) == 0 {
		countNum = 10
	} else {
		countNum, err = strconv.Atoi(count)
		if err != nil {
			log.Errorf("Get parameters error--count, err: %v", err)
			ra.CustomAbort(http.StatusBadRequest, "bad request of count")
		}
		if countNum <= 0 {
			log.Warning("count must be a positive integer")
			ra.CustomAbort(http.StatusBadRequest, "count is 0 or negative")
		}
	}
	repos, err := dao.GetTopRepos(countNum)
	if err != nil {
		log.Errorf("error occured in get top 10 repos: %v", err)
		ra.CustomAbort(http.StatusInternalServerError, "internal server error")
	}
	ra.Data["json"] = repos
	ra.ServeJSON()
}

func newRepositoryClient(endpoint string, insecure bool, username, password, repository, scopeType, scopeName string,
	scopeActions ...string) (*registry.Repository, error) {

	credential := auth.NewBasicAuthCredential(username, password)
	authorizer := auth.NewStandardTokenAuthorizer(credential, insecure, scopeType, scopeName, scopeActions...)

	store, err := auth.NewAuthorizerStore(endpoint, insecure, authorizer)
	if err != nil {
		return nil, err
	}

	client, err := registry.NewRepositoryWithModifiers(repository, endpoint, insecure, store)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func getProjectName(repository string) string {
	project := ""
	if strings.Contains(repository, "/") {
		project = repository[0:strings.LastIndex(repository, "/")]
	}
	return project
}
