// Copyright (c) 2017, 2019, Oracle and/or its affiliates. All rights reserved.

package provider

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/terraform/helper/schema"

	oci_common "github.com/oracle/oci-go-sdk/common"
	oci_core "github.com/oracle/oci-go-sdk/core"
)

func RemotePeeringConnectionResource() *schema.Resource {
	return &schema.Resource{
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},
		Timeouts: DefaultTimeout,
		Create:   createRemotePeeringConnection,
		Read:     readRemotePeeringConnection,
		Update:   updateRemotePeeringConnection,
		Delete:   deleteRemotePeeringConnection,
		Schema: map[string]*schema.Schema{
			// Required
			"compartment_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"drg_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			// Optional
			"display_name": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			// @CODEGEN peer_id and peer_region_name moved from computed to optional as they are required for the connect action
			"peer_id": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ForceNew:     true,
				ValidateFunc: validateNotEmptyString(), //Don't allow empty string, it results in a terraform error when switching from valid value to empty string
			},
			"peer_region_name": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ForceNew:     true,
				ValidateFunc: validateNotEmptyString(), //Don't allow empty string, it results in a terraform error when switching from valid value to empty string
			},

			// Computed
			"is_cross_tenancy_peering": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			"peer_tenancy_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"peering_status": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"state": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"time_created": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func createRemotePeeringConnection(d *schema.ResourceData, m interface{}) error {
	sync := &RemotePeeringConnectionResourceCrud{}
	sync.D = d
	sync.Client = m.(*OracleClients).virtualNetworkClient

	err := CreateResource(d, sync)
	if err != nil {
		return err
	}

	//This needs to be here rather than in the Create() because we want the resource to finish provisioning and set to the statefile before we connect
	return sync.ConnectRemotePeeringConnection()
}

func readRemotePeeringConnection(d *schema.ResourceData, m interface{}) error {
	sync := &RemotePeeringConnectionResourceCrud{}
	sync.D = d
	sync.Client = m.(*OracleClients).virtualNetworkClient

	return ReadResource(sync)
}

func updateRemotePeeringConnection(d *schema.ResourceData, m interface{}) error {
	sync := &RemotePeeringConnectionResourceCrud{}
	sync.D = d
	sync.Client = m.(*OracleClients).virtualNetworkClient

	return UpdateResource(d, sync)
}

func deleteRemotePeeringConnection(d *schema.ResourceData, m interface{}) error {
	sync := &RemotePeeringConnectionResourceCrud{}
	sync.D = d
	sync.Client = m.(*OracleClients).virtualNetworkClient
	sync.DisableNotFoundRetries = true

	return DeleteResource(d, sync)
}

type RemotePeeringConnectionResourceCrud struct {
	BaseCrud
	Client                 *oci_core.VirtualNetworkClient
	Res                    *oci_core.RemotePeeringConnection
	DisableNotFoundRetries bool
}

func (s *RemotePeeringConnectionResourceCrud) ID() string {
	return *s.Res.Id
}

func (s *RemotePeeringConnectionResourceCrud) CreatedPending() []string {
	return []string{
		string(oci_core.RemotePeeringConnectionLifecycleStateProvisioning),
	}
}

func (s *RemotePeeringConnectionResourceCrud) CreatedTarget() []string {
	return []string{
		string(oci_core.RemotePeeringConnectionLifecycleStateAvailable),
	}
}

func (s *RemotePeeringConnectionResourceCrud) DeletedPending() []string {
	return []string{
		string(oci_core.RemotePeeringConnectionLifecycleStateTerminating),
	}
}

func (s *RemotePeeringConnectionResourceCrud) DeletedTarget() []string {
	return []string{
		string(oci_core.RemotePeeringConnectionLifecycleStateTerminated),
	}
}

func (s *RemotePeeringConnectionResourceCrud) ConnectRemotePeeringConnection() error {
	if s.Res == nil || s.Res.Id == nil {
		return fmt.Errorf("CreateRemotePeeringConnection returned a nil RemotePeeringConnection or a RemotePeeringConnection without an ID")
	}

	peerId, ok := s.D.GetOkExists("peer_id")
	if !ok {
		return nil
	}

	connectRequest := oci_core.ConnectRemotePeeringConnectionsRequest{}

	tmp := peerId.(string)
	connectRequest.PeerId = &tmp

	connectRequest.RemotePeeringConnectionId = s.Res.Id

	if peerRegionName, ok := s.D.GetOkExists("peer_region_name"); ok {
		tmp := peerRegionName.(string)
		connectRequest.PeerRegionName = &tmp
	}

	connectRequest.RequestMetadata.RetryPolicy = getRetryPolicy(s.DisableNotFoundRetries, "core")

	_, err := s.Client.ConnectRemotePeeringConnections(context.Background(), connectRequest)
	if err != nil {
		// we set peer_id to "" so that terraform detects a forceNew change on the next apply and the user can try the connection again
		s.D.Set("peer_id", "")
		return err
	}

	request := oci_core.GetRemotePeeringConnectionRequest{}

	tmpId := s.D.Id()
	request.RemotePeeringConnectionId = &tmpId

	request.RequestMetadata.RetryPolicy = getRemotePeeringConnectionRetryPolicy(s.D.Timeout(schema.TimeoutCreate))

	response, getError := s.Client.GetRemotePeeringConnection(context.Background(), request)
	if getError != nil {
		log.Printf("[DEBUG] Get error while waiting for peering connection to finish: %+v", getError)
		return getError
	}
	s.Res = &response.RemotePeeringConnection
	if response.RemotePeeringConnection.PeeringStatus != oci_core.RemotePeeringConnectionPeeringStatusPeered {
		s.D.Set("peer_id", "")
		return fmt.Errorf("unexpected Peering Status `%s` after trying to connect to the peer Remote Peering Connection (RPC). Make sure the peering_status of the peer RPC is not REVOKED", string(response.RemotePeeringConnection.PeeringStatus))
	}

	if err := s.SetData(); err != nil {
		return err
	}

	return nil
}

func (s *RemotePeeringConnectionResourceCrud) Create() error {
	request := oci_core.CreateRemotePeeringConnectionRequest{}

	if compartmentId, ok := s.D.GetOkExists("compartment_id"); ok {
		tmp := compartmentId.(string)
		request.CompartmentId = &tmp
	}

	if displayName, ok := s.D.GetOkExists("display_name"); ok {
		tmp := displayName.(string)
		request.DisplayName = &tmp
	}

	if drgId, ok := s.D.GetOkExists("drg_id"); ok {
		tmp := drgId.(string)
		request.DrgId = &tmp
	}

	request.RequestMetadata.RetryPolicy = getRetryPolicy(s.DisableNotFoundRetries, "core")

	response, err := s.Client.CreateRemotePeeringConnection(context.Background(), request)
	if err != nil {
		return err
	}

	s.Res = &response.RemotePeeringConnection
	return nil
}

func (s *RemotePeeringConnectionResourceCrud) Get() error {
	request := oci_core.GetRemotePeeringConnectionRequest{}

	tmp := s.D.Id()
	request.RemotePeeringConnectionId = &tmp

	request.RequestMetadata.RetryPolicy = getRetryPolicy(s.DisableNotFoundRetries, "core")

	response, err := s.Client.GetRemotePeeringConnection(context.Background(), request)
	if err != nil {
		return err
	}

	s.Res = &response.RemotePeeringConnection
	return nil
}

func (s *RemotePeeringConnectionResourceCrud) Update() error {
	request := oci_core.UpdateRemotePeeringConnectionRequest{}

	if displayName, ok := s.D.GetOkExists("display_name"); ok {
		tmp := displayName.(string)
		request.DisplayName = &tmp
	}

	tmp := s.D.Id()
	request.RemotePeeringConnectionId = &tmp

	request.RequestMetadata.RetryPolicy = getRetryPolicy(s.DisableNotFoundRetries, "core")

	response, err := s.Client.UpdateRemotePeeringConnection(context.Background(), request)
	if err != nil {
		return err
	}

	s.Res = &response.RemotePeeringConnection
	return nil
}

func (s *RemotePeeringConnectionResourceCrud) Delete() error {
	request := oci_core.DeleteRemotePeeringConnectionRequest{}

	tmp := s.D.Id()
	request.RemotePeeringConnectionId = &tmp

	request.RequestMetadata.RetryPolicy = getRetryPolicy(s.DisableNotFoundRetries, "core")

	_, err := s.Client.DeleteRemotePeeringConnection(context.Background(), request)
	return err
}

func (s *RemotePeeringConnectionResourceCrud) SetData() error {
	if s.Res.CompartmentId != nil {
		s.D.Set("compartment_id", *s.Res.CompartmentId)
	}

	if s.Res.DisplayName != nil {
		s.D.Set("display_name", *s.Res.DisplayName)
	}

	if s.Res.DrgId != nil {
		s.D.Set("drg_id", *s.Res.DrgId)
	}

	if s.Res.IsCrossTenancyPeering != nil {
		s.D.Set("is_cross_tenancy_peering", *s.Res.IsCrossTenancyPeering)
	}

	if s.Res.PeerId != nil {
		s.D.Set("peer_id", *s.Res.PeerId)
	}

	if s.Res.PeerRegionName != nil {
		s.D.Set("peer_region_name", *s.Res.PeerRegionName)
	}

	if s.Res.PeerTenancyId != nil {
		s.D.Set("peer_tenancy_id", *s.Res.PeerTenancyId)
	}

	s.D.Set("peering_status", s.Res.PeeringStatus)

	s.D.Set("state", s.Res.LifecycleState)

	if s.Res.TimeCreated != nil {
		s.D.Set("time_created", s.Res.TimeCreated.String())
	}

	return nil
}

func getRemotePeeringConnectionRetryPolicy(timeout time.Duration) *oci_common.RetryPolicy {
	startTime := time.Now()
	// wait for peering status to not be Pending
	return &oci_common.RetryPolicy{
		ShouldRetryOperation: func(response oci_common.OCIOperationResponse) bool {
			if shouldRetry(response, false, "core", startTime) {
				return true
			}
			if getRemotePeeringConnectionResponse, ok := response.Response.(oci_core.GetRemotePeeringConnectionResponse); ok {
				if getRemotePeeringConnectionResponse.PeeringStatus == oci_core.RemotePeeringConnectionPeeringStatusPending {
					timeWaited := getElapsedRetryDuration(startTime)
					return timeWaited < timeout
				}
			}
			return false
		},
		NextDuration: func(response oci_common.OCIOperationResponse) time.Duration {
			return getRetryBackoffDuration(response, false, "core", startTime)
		},
		MaximumNumberAttempts: 0,
	}
}
