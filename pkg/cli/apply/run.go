// Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	workloadutils "github.com/silogen/kaiwo/pkg/workloads2/utils"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"github.com/imdario/mergo"

	"github.com/silogen/kaiwo/pkg/workloads2"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/k8s"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

type WorkloadApplier interface {
	// LoadFromPath loads supplementary information from path, which is a local path
	LoadFromPath(path string) error

	// FromCliFlags initializes the applier from CLI flags
	FromCliFlags(flags workloads2.CLIFlags)

	GetObject() client.Object

	GetInvoker(ctx context.Context, scheme *runtime.Scheme) (workloadutils.CommandInvoker, error)
}

func Apply(applier WorkloadApplier, flags workloads2.CLIFlags) error {
	var baseManifest client.Object = nil

	scheme, err := k8s.GetScheme()
	if err != nil {
		return err
	}

	applier.FromCliFlags(flags)

	if flags.BaseManifestPath != "" {
		baseManifest = applier.GetObject().DeepCopyObject().(client.Object)
		if err := ReadBaseManifest(flags.BaseManifestPath, &scheme, baseManifest); err != nil {
			return fmt.Errorf("error reading base manifest: %v", err)
		}
	}

	if flags.Path != "" {
		// TODO resolve if path points to a git repository?
		if err := applier.LoadFromPath(flags.Path); err != nil {
			return fmt.Errorf("error loading manifest: %v", err)
		}
	}

	manifest := applier.GetObject()

	if baseManifest != nil {
		if err := mergo.Merge(manifest, baseManifest); err != nil {
			return fmt.Errorf("failed to merge generated manifest with base manifest: %w", err)
		}
	}

	ctx := context.TODO()
	k8sClient, err := k8s.GetClient()
	if err != nil {
		return fmt.Errorf("error getting k8s client: %v", err)
	}

	if flags.DryRun {
		if err := ApplyServerSideDryRun(ctx, k8sClient, manifest); err != nil {
			return fmt.Errorf("error applying server side dry-run: %v", err)
		}
	} else if flags.PrintOutput {
		if err := ApplyLocalPrint(manifest, scheme); err != nil {
			return fmt.Errorf("error applying local-print: %v", err)
		}
	} else if flags.Preview {
		invoker, err := applier.GetInvoker(ctx, &scheme)
		if err != nil {
			return fmt.Errorf("failed to get invoker: %w", err)
		}
		resources, err := invoker.BuildAllResources()
		if err != nil {
			return fmt.Errorf("failed to build resources: %w", err)
		}
		for _, resource := range resources {
			if err := ApplyLocalPrint(resource, scheme); err != nil {
				return fmt.Errorf("failed to apply resource %s: %w", resource.GetName(), err)
			}
			fmt.Println("---")
		}
	} else {
		if err := ApplyCreate(ctx, k8sClient, manifest); err != nil {
			return fmt.Errorf("error creating resource: %v", err)
		}
	}

	return nil
}

func ReadBaseManifest(uri string, scheme *runtime.Scheme, into client.Object) error {
	var contents []byte

	if strings.HasPrefix(uri, "git:") {
		return fmt.Errorf("git remotes not supported yet") // TODO implement
	} else if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		return fmt.Errorf("http remotes not supported yet") // TODO implement
	} else {
		fileContents, err := os.ReadFile(uri)
		if err != nil {
			return fmt.Errorf("failed to read base manifest: %v", err)
		}
		contents = fileContents
	}

	decoder := serializer.NewCodecFactory(scheme).UniversalDeserializer()

	if _, _, err := decoder.Decode(contents, nil, into); err != nil {
		return fmt.Errorf("failed to decode manifest: %w", err)
	}

	return nil
}

func ApplyServerSideDryRun(ctx context.Context, k8sClient client.Client, manifest client.Object) error {
	if manifest.GetName() == "" || manifest.GetNamespace() == "" {
		return fmt.Errorf("manifest must have a name and namespace")
	}

	options := &client.PatchOptions{
		Force:        baseutils.Pointer(true),
		FieldManager: "dry-run-manager",
	}

	err := k8sClient.Patch(ctx, manifest, client.Apply, options, &client.DryRunAll)
	if err != nil {
		return fmt.Errorf("failed to apply dry-run: %w", err)
	}

	logrus.Infof("manifest '%s' dry run succeeded", manifest.GetName())

	return nil
}

func ApplyCreate(ctx context.Context, k8sClient client.Client, manifest client.Object) error {
	obj := manifest.DeepCopyObject().(client.Object)

	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get object: %w", err)
		}
	} else {
		logrus.Info("Resource already exists, skipping apply")
		return nil
	}

	if err := k8sClient.Create(ctx, manifest); err != nil {
		return err
	}

	return nil
}

func ApplyLocalPrint(manifest client.Object, scheme runtime.Scheme) error {
	cleanedResource, err := k8s.MinimalizeAndConvertToYAML(&scheme, manifest)
	if err != nil {
		return err
	}

	fmt.Print(cleanedResource)

	return nil
}
