package config

import (
	"context"
	"errors"

	"github.com/khulnasoft/meshkit/utils"
	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	namespace = "meshplay"           // Namespace for the Custom Resource
	crName    = "meshplay-meshsync"  // Name of the custom resource
	version   = "v1alpha1"          // Version of the Custom Resource
	group     = "meshplay.khulnasoft.com" //Group for the Custom Resource
	resource  = "meshsyncs"         //Name of the Resource
)

func GetMeshsyncCRDConfigs(dyClient dynamic.Interface) (*MeshsyncConfig, error) {
	// initialize the group version resource to access the custom resource
	gvr := schema.GroupVersionResource{Version: version, Group: group, Resource: resource}

	// make a call to get the custom resource
	crd, err := dyClient.Resource(gvr).Namespace(namespace).Get(context.TODO(), crName, metav1.GetOptions{})

	if err != nil {
		return nil, ErrInitConfig(err)
	}

	if crd == nil {
		return nil, ErrInitConfig(errors.New("Custom Resource is nil"))
	}

	spec := crd.Object["spec"]
	specMap, ok := spec.(map[string]interface{})
	if !ok {
		return nil, ErrInitConfig(errors.New("Unable to convert spec to map"))
	}
	configObj := specMap["watch-list"]
	if configObj == nil {
		return nil, ErrInitConfig(errors.New("Custom Resource does not have Meshsync Configs"))
	}
	configStr, err := utils.Marshal(configObj)
	if err != nil {
		return nil, ErrInitConfig(err)
	}

	configMap := corev1.ConfigMap{}
	err = utils.Unmarshal(string(configStr), &configMap)

	if err != nil {
		return nil, ErrInitConfig(err)
	}

	// populate the required configs
	meshsyncConfig, err := PopulateConfigs(configMap)

	if err != nil {
		return nil, ErrInitConfig(err)
	}
	return meshsyncConfig, nil
}

// PopulateConfigs compares the default configs and the whitelist and blacklist
func PopulateConfigs(configMap corev1.ConfigMap) (*MeshsyncConfig, error) {
	meshsyncConfig := &MeshsyncConfig{}

	if _, ok := configMap.Data["blacklist"]; ok {
		if len(configMap.Data["blacklist"]) > 0 {
			err := utils.Unmarshal(configMap.Data["blacklist"], &meshsyncConfig.BlackList)
			if err != nil {
				return nil, ErrInitConfig(err)
			}
		}
	}

	if _, ok := configMap.Data["whitelist"]; ok {
		if len(configMap.Data["whitelist"]) > 0 {
			err := utils.Unmarshal(configMap.Data["whitelist"], &meshsyncConfig.WhiteList)
			if err != nil {
				return nil, ErrInitConfig(err)
			}
		}
	}

	// ensure that atleast one of whitelist or blacklist has been supplied
	if len(meshsyncConfig.BlackList) == 0 && len(meshsyncConfig.WhiteList) == 0 {
		return nil, ErrInitConfig(errors.New("Both whitelisted and blacklisted resources missing"))
	}

	// ensure that only one of whitelist or blacklist has been supplied
	if len(meshsyncConfig.BlackList) != 0 && len(meshsyncConfig.WhiteList) != 0 {
		return nil, ErrInitConfig(errors.New("Both whitelisted and blacklisted resources not currently supported"))
	}

	// Handle global resources
	globalPipelines := make(PipelineConfigs, 0)
	localPipelines := make(PipelineConfigs, 0)

	if len(meshsyncConfig.WhiteList) != 0 {
		for _, v := range Pipelines[GlobalResourceKey] {
			if idx := slices.IndexFunc(meshsyncConfig.WhiteList, func(c ResourceConfig) bool { return c.Resource == v.Name }); idx != -1 {
				config := meshsyncConfig.WhiteList[idx]
				v.Events = config.Events
				globalPipelines = append(globalPipelines, v)
			}
		}
		if len(globalPipelines) > 0 {
			meshsyncConfig.Pipelines = map[string]PipelineConfigs{}
			meshsyncConfig.Pipelines[GlobalResourceKey] = globalPipelines
		}

		// Handle local resources
		for _, v := range Pipelines[LocalResourceKey] {
			if idx := slices.IndexFunc(meshsyncConfig.WhiteList, func(c ResourceConfig) bool { return c.Resource == v.Name }); idx != -1 {
				config := meshsyncConfig.WhiteList[idx]
				v.Events = config.Events
				localPipelines = append(localPipelines, v)
			}
		}

		if len(localPipelines) > 0 {
			if meshsyncConfig.Pipelines == nil {
				meshsyncConfig.Pipelines = make(map[string]PipelineConfigs)
			}
			meshsyncConfig.Pipelines[LocalResourceKey] = localPipelines
		}

	} else {

		for _, v := range Pipelines[GlobalResourceKey] {
			if idx := slices.IndexFunc(meshsyncConfig.BlackList, func(c string) bool { return c == v.Name }); idx == -1 {
				v.Events = DefaultEvents
				globalPipelines = append(globalPipelines, v)
			}
		}
		if len(globalPipelines) > 0 {
			meshsyncConfig.Pipelines = map[string]PipelineConfigs{}
			meshsyncConfig.Pipelines[GlobalResourceKey] = globalPipelines
		}

		// Handle local resources
		for _, v := range Pipelines[LocalResourceKey] {
			if idx := slices.IndexFunc(meshsyncConfig.BlackList, func(c string) bool { return c == v.Name }); idx == -1 {
				v.Events = DefaultEvents
				localPipelines = append(localPipelines, v)
			}
		}

		if len(localPipelines) > 0 {
			if meshsyncConfig.Pipelines == nil {
				meshsyncConfig.Pipelines = make(map[string]PipelineConfigs)
			}
			meshsyncConfig.Pipelines[LocalResourceKey] = localPipelines
		}
	}

	return meshsyncConfig, nil
}
