package agent

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/hcl"
	"github.com/hashicorp/hcl/hcl/ast"
	"github.com/mitchellh/mapstructure"

	uconf "udup/internal/config"
)

// ParseConfigFile parses the given path as a config file.
func ParseConfigFile(path string) (*Config, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	config, err := ParseConfig(f)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// ParseConfig parses the config from the given io.Reader.
//
// Due to current internal limitations, the entire contents of the
// io.Reader will be copied into memory first before parsing.
func ParseConfig(r io.Reader) (*Config, error) {
	// Copy the reader into an in-memory buffer first since HCL requires it.
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, err
	}

	// Parse the buffer
	root, err := hcl.Parse(buf.String())
	if err != nil {
		return nil, fmt.Errorf("error parsing: %s", err)
	}
	buf.Reset()

	// Top-level item should be a list
	list, ok := root.Node.(*ast.ObjectList)
	if !ok {
		return nil, fmt.Errorf("error parsing: root should be an object")
	}

	var config Config
	if err := parseConfig(&config, list); err != nil {
		return nil, fmt.Errorf("error parsing 'config': %v", err)
	}

	return &config, nil
}

func parseConfig(result *Config, list *ast.ObjectList) error {
	// Check for invalid keys
	valid := []string{
		"region",
		"datacenter",
		"name",
		"data_dir",
		"log_level",
		"bind_addr",
		"ports",
		"addresses",
		"advertise",
		"client",
		"server",
		"metric",
		"leave_on_interrupt",
		"leave_on_terminate",
		"consul",
		"nats",
		"http_api_response_headers",
	}
	if err := checkHCLKeys(list, valid); err != nil {
		return multierror.Prefix(err, "config:")
	}

	// Decode the full thing into a map[string]interface for ease
	var m map[string]interface{}
	if err := hcl.DecodeObject(&m, list); err != nil {
		return err
	}
	delete(m, "ports")
	delete(m, "addresses")
	delete(m, "advertise")
	delete(m, "client")
	delete(m, "server")
	delete(m, "metric")
	delete(m, "consul")
	delete(m, "nats")
	delete(m, "http_api_response_headers")

	// Decode the rest
	if err := mapstructure.WeakDecode(m, result); err != nil {
		return err
	}

	// Parse ports
	if o := list.Filter("ports"); len(o.Items) > 0 {
		if err := parsePorts(&result.Ports, o); err != nil {
			return multierror.Prefix(err, "ports ->")
		}
	}

	// Parse addresses
	if o := list.Filter("addresses"); len(o.Items) > 0 {
		if err := parseAddresses(&result.Addresses, o); err != nil {
			return multierror.Prefix(err, "addresses ->")
		}
	}

	// Parse advertise
	if o := list.Filter("advertise"); len(o.Items) > 0 {
		if err := parseAdvertise(&result.AdvertiseAddrs, o); err != nil {
			return multierror.Prefix(err, "advertise ->")
		}
	}

	// Parse client config
	if o := list.Filter("client"); len(o.Items) > 0 {
		if err := parseClient(&result.Client, o); err != nil {
			return multierror.Prefix(err, "client ->")
		}
	}

	// Parse server config
	if o := list.Filter("server"); len(o.Items) > 0 {
		if err := parseServer(&result.Server, o); err != nil {
			return multierror.Prefix(err, "server ->")
		}
	}

	// Parse metric config
	if o := list.Filter("metric"); len(o.Items) > 0 {
		if err := parseMetric(&result.Metric, o); err != nil {
			return multierror.Prefix(err, "metric ->")
		}
	}

	// Parse the consul config
	if o := list.Filter("consul"); len(o.Items) > 0 {
		if err := parseConsulConfig(&result.Consul, o); err != nil {
			return multierror.Prefix(err, "consul ->")
		}
	}

	// Parse the nats config
	if o := list.Filter("nats"); len(o.Items) > 0 {
		if err := parseNatsConfig(&result.Nats, o); err != nil {
			return multierror.Prefix(err, "nats ->")
		}
	}

	// Parse out http_api_response_headers fields. These are in HCL as a list so
	// we need to iterate over them and merge them.
	if headersO := list.Filter("http_api_response_headers"); len(headersO.Items) > 0 {
		for _, o := range headersO.Elem().Items {
			var m map[string]interface{}
			if err := hcl.DecodeObject(&m, o.Val); err != nil {
				return err
			}
			if err := mapstructure.WeakDecode(m, &result.HTTPAPIResponseHeaders); err != nil {
				return err
			}
		}
	}

	return nil
}

func parsePorts(result **Ports, list *ast.ObjectList) error {
	list = list.Elem()
	if len(list.Items) > 1 {
		return fmt.Errorf("only one 'ports' block allowed")
	}

	// Get our ports object
	listVal := list.Items[0].Val

	// Check for invalid keys
	valid := []string{
		"http",
		"rpc",
		"serf",
	}
	if err := checkHCLKeys(listVal, valid); err != nil {
		return err
	}

	var m map[string]interface{}
	if err := hcl.DecodeObject(&m, listVal); err != nil {
		return err
	}

	var ports Ports
	if err := mapstructure.WeakDecode(m, &ports); err != nil {
		return err
	}
	*result = &ports
	return nil
}

func parseAddresses(result **Addresses, list *ast.ObjectList) error {
	list = list.Elem()
	if len(list.Items) > 1 {
		return fmt.Errorf("only one 'addresses' block allowed")
	}

	// Get our addresses object
	listVal := list.Items[0].Val

	// Check for invalid keys
	valid := []string{
		"http",
		"rpc",
		"serf",
	}
	if err := checkHCLKeys(listVal, valid); err != nil {
		return err
	}

	var m map[string]interface{}
	if err := hcl.DecodeObject(&m, listVal); err != nil {
		return err
	}

	var addresses Addresses
	if err := mapstructure.WeakDecode(m, &addresses); err != nil {
		return err
	}
	*result = &addresses
	return nil
}

func parseAdvertise(result **AdvertiseAddrs, list *ast.ObjectList) error {
	list = list.Elem()
	if len(list.Items) > 1 {
		return fmt.Errorf("only one 'advertise' block allowed")
	}

	// Get our advertise object
	listVal := list.Items[0].Val

	// Check for invalid keys
	valid := []string{
		"http",
		"rpc",
		"serf",
	}
	if err := checkHCLKeys(listVal, valid); err != nil {
		return err
	}

	var m map[string]interface{}
	if err := hcl.DecodeObject(&m, listVal); err != nil {
		return err
	}

	var advertise AdvertiseAddrs
	if err := mapstructure.WeakDecode(m, &advertise); err != nil {
		return err
	}
	*result = &advertise
	return nil
}

func parseClient(result **ClientConfig, list *ast.ObjectList) error {
	list = list.Elem()
	if len(list.Items) > 1 {
		return fmt.Errorf("only one 'client' block allowed")
	}

	// Get our client object
	obj := list.Items[0]

	// Value should be an object
	var listVal *ast.ObjectList
	if ot, ok := obj.Val.(*ast.ObjectType); ok {
		listVal = ot.List
	} else {
		return fmt.Errorf("client value: should be an object")
	}

	// Check for invalid keys
	valid := []string{
		"enabled",
		"state_dir",
		"servers",
		"stats",
		"no_host_uuid",
	}
	if err := checkHCLKeys(listVal, valid); err != nil {
		return err
	}

	var m map[string]interface{}
	if err := hcl.DecodeObject(&m, listVal); err != nil {
		return err
	}

	delete(m, "stats")

	var config ClientConfig
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
		WeaklyTypedInput: true,
		Result:           &config,
	})
	if err != nil {
		return err
	}
	if err := dec.Decode(m); err != nil {
		return err
	}

	*result = &config
	return nil
}

func parseServer(result **ServerConfig, list *ast.ObjectList) error {
	list = list.Elem()
	if len(list.Items) > 1 {
		return fmt.Errorf("only one 'server' block allowed")
	}

	// Get our server object
	obj := list.Items[0]

	// Value should be an object
	var listVal *ast.ObjectList
	if ot, ok := obj.Val.(*ast.ObjectType); ok {
		listVal = ot.List
	} else {
		return fmt.Errorf("client value: should be an object")
	}

	// Check for invalid keys
	valid := []string{
		"enabled",
		"bootstrap_expect",
		"data_dir",
		"num_schedulers",
		"enabled_schedulers",
		"heartbeat_grace",
		"start_join",
		"retry_join",
		"retry_max",
		"retry_interval",
	}
	if err := checkHCLKeys(listVal, valid); err != nil {
		return err
	}

	var m map[string]interface{}
	if err := hcl.DecodeObject(&m, listVal); err != nil {
		return err
	}

	var config ServerConfig
	if err := mapstructure.WeakDecode(m, &config); err != nil {
		return err
	}

	*result = &config
	return nil
}

func parseMetric(result **Metric, list *ast.ObjectList) error {
	list = list.Elem()
	if len(list.Items) > 1 {
		return fmt.Errorf("only one 'metric' block allowed")
	}

	// Get our metric object
	listVal := list.Items[0].Val

	// Check for invalid keys
	valid := []string{
		"statsite_address",
		"statsd_address",
		"disable_hostname",
		"use_node_name",
		"collection_interval",
		"publish_allocation_metrics",
		"publish_node_metrics",
		"datadog_address",
		"circonus_api_token",
		"circonus_api_app",
		"circonus_api_url",
		"circonus_submission_interval",
		"circonus_submission_url",
		"circonus_check_id",
		"circonus_check_force_metric_activation",
		"circonus_check_instance_id",
		"circonus_check_search_tag",
		"circonus_check_display_name",
		"circonus_check_tags",
		"circonus_broker_id",
		"circonus_broker_select_tag",
	}
	if err := checkHCLKeys(listVal, valid); err != nil {
		return err
	}

	var m map[string]interface{}
	if err := hcl.DecodeObject(&m, listVal); err != nil {
		return err
	}

	var metric Metric
	if err := mapstructure.WeakDecode(m, &metric); err != nil {
		return err
	}
	if metric.CollectionInterval != "" {
		if dur, err := time.ParseDuration(metric.CollectionInterval); err != nil {
			return fmt.Errorf("error parsing value of %q: %v", "collection_interval", err)
		} else {
			metric.collectionInterval = dur
		}
	}
	*result = &metric
	return nil
}

func parseConsulConfig(result **uconf.ConsulConfig, list *ast.ObjectList) error {
	list = list.Elem()
	if len(list.Items) > 1 {
		return fmt.Errorf("only one 'consul' block allowed")
	}

	// Get our Consul object
	listVal := list.Items[0].Val

	// Check for invalid keys
	valid := []string{
		"address",
		"auth",
		"auto_advertise",
		"ca_file",
		"cert_file",
		"checks_use_advertise",
		"client_auto_join",
		"client_service_name",
		"key_file",
		"server_auto_join",
		"server_service_name",
		"ssl",
		"timeout",
		"token",
		"verify_ssl",
	}

	if err := checkHCLKeys(listVal, valid); err != nil {
		return err
	}

	var m map[string]interface{}
	if err := hcl.DecodeObject(&m, listVal); err != nil {
		return err
	}

	consulConfig := uconf.DefaultConsulConfig()
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
		WeaklyTypedInput: true,
		Result:           &consulConfig,
	})
	if err != nil {
		return err
	}
	if err := dec.Decode(m); err != nil {
		return err
	}

	*result = consulConfig
	return nil
}

func parseNatsConfig(result **uconf.NatsConfig, list *ast.ObjectList) error {
	list = list.Elem()
	if len(list.Items) > 1 {
		return fmt.Errorf("only one 'consul' block allowed")
	}

	// Get our Consul object
	listVal := list.Items[0].Val

	// Check for invalid keys
	valid := []string{
		"address",
		"store_type",
		"file_store_dir",
	}

	if err := checkHCLKeys(listVal, valid); err != nil {
		return err
	}

	var m map[string]interface{}
	if err := hcl.DecodeObject(&m, listVal); err != nil {
		return err
	}

	natsConfig := uconf.DefaultNatsConfig()
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
		WeaklyTypedInput: true,
		Result:           &natsConfig,
	})
	if err != nil {
		return err
	}
	if err := dec.Decode(m); err != nil {
		return err
	}

	*result = natsConfig
	return nil
}

func checkHCLKeys(node ast.Node, valid []string) error {
	var list *ast.ObjectList
	switch n := node.(type) {
	case *ast.ObjectList:
		list = n
	case *ast.ObjectType:
		list = n.List
	default:
		return fmt.Errorf("cannot check HCL keys of type %T", n)
	}

	validMap := make(map[string]struct{}, len(valid))
	for _, v := range valid {
		validMap[v] = struct{}{}
	}

	var result error
	for _, item := range list.Items {
		key := item.Keys[0].Token.Value().(string)
		if _, ok := validMap[key]; !ok {
			result = multierror.Append(result, fmt.Errorf(
				"invalid key: %s", key))
		}
	}

	return result
}