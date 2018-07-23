/*
Copyright 2018 The Kubernetes Authors.

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

package main

import (
	"flag"

	"syscall"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/external-storage/lib/controller"
	"github.com/kubernetes-incubator/external-storage/openebs/pkg/provisioner"
	mayav1 "github.com/kubernetes-incubator/external-storage/openebs/types/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	provisionerName = "openebs.io/provisioner-iscsi"
	// CASTemplateFeatureGateENVK is the ENV key to fetch cas template feature gate
	// i.e. if cas template based provisioning is enabled or disabled
	CASTemplateFeatureGateENVK = "OPENEBS_IO_CAS_TEMPLATE_FEATURE_GATE"
)

func main() {
	syscall.Umask(0)

	flag.Parse()
	flag.Set("logtostderr", "true")
	var (
		config     *rest.Config
		err        error
		k8sMaster  = mayav1.K8sMasterENV()
		kubeConfig = mayav1.KubeConfigENV()
	)
	if len(k8sMaster) != 0 || len(kubeConfig) != 0 {
		glog.Infof("Build client config using k8s Master's Address: '%s' or Kubeconfig: '%s' \n", k8sMaster, kubeConfig)
		config, err = clientcmd.BuildConfigFromFlags(k8sMaster, kubeConfig)
	} else {
		// Create an InClusterConfig and use it to create a client for the controller
		// to use to communicate with Kubernetes
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		glog.Fatalf("Failed to create config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("Failed to create client: %v", err)
	}

	// The controller needs to know what the server version is because out-of-tree
	// provisioners aren't officially supported until 1.5
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		glog.Fatalf("Error getting server version: %v", err)
	}

	// Create the provisioner: it implements the Provisioner interface expected by
	// the controller
	var openEBSProvisioner controller.Provisioner
	feature, err := CASTemplateFeatureGate()
	if err != nil {
		glog.Fatalf("failed to fetch CAS template feature gate: %v", err)
	}
	if feature {
		glog.Infof("Using CAS template feature gate for volume provisioning")

		openEBSProvisioner, err = provisioner.NewOpenEBSCASProvisioner(clientset)
		if err != nil {
			glog.Fatalf("Error creating Openebs provisioner: %v", err)
		}
	} else {
		openEBSProvisioner = provisioner.NewOpenEBSProvisioner(clientset)
	}

	// Start the provision controller which will dynamically provision OpenEBS PVs
	pc := controller.NewProvisionController(
		clientset,
		provisionerName,
		openEBSProvisioner,
		serverVersion.GitVersion,
	)
	// Run starts all of controller's control loops
	pc.Run(wait.NeverStop)
}
