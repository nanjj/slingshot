package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/amap"
	cli "github.com/nanjj/slingshot/internal/cmd"
	"github.com/nanjj/slingshot/internal/config"
	"github.com/nanjj/slingshot/internal/i18n"
	u "github.com/nanjj/slingshot/internal/usage"
)

// amap 子命令语法定义。
// 注意: cobra 已处理子命令名 (search/around/...), 这里只定义子命令后的参数。
var (
	amapSearchUsage   = u.Usage{u.Placeholder("keywords"), u.Placeholder("city").Optional()}
	amapAroundUsage   = u.Usage{u.Placeholder("keywords"), u.Placeholder("location"), u.Placeholder("radius").Optional()}
	amapDetailUsage   = u.Usage{u.Placeholder("id")}
	amapGeoUsage      = u.Usage{u.Placeholder("address")}
	amapRegeoUsage    = u.Usage{u.Placeholder("location")}
	amapRouteUsage    = u.Usage{u.Placeholder("origin"), u.Placeholder("destination")}
	amapTransitUsage  = u.Usage{u.Placeholder("origin"), u.Placeholder("destination"), u.Placeholder("city"), u.Placeholder("cityd")}
	amapDistanceUsage = u.Usage{u.Placeholder("origins"), u.Placeholder("destination"), u.Placeholder("type").Optional()}
	amapIPUsage       = u.Usage{u.Placeholder("ip").Optional()}
)

// cmdAmap 实现 "slingshot amap" 子命令。
type cmdAmap struct {
	global *cmdGlobal
}

func (c *cmdAmap) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "amap"
	cmd.Short = i18n.G("Query Amap (高德地图) POI, geocoding and routing")
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		i18n.G(`Query the Amap (高德地图) MCP server: POI search, geocoding,
route planning, distance measurement and IP location.

Coordinates are GCJ-02 and written as "经度,纬度" (e.g. 111.772234,40.853779).

Configuration:
  slingshot config set amap.key <key>   # or export AMAP_KEY

Subcommands:
  search    <keywords> [city]                 POI keyword search (city-limited)
  around    <keywords> <location> [radius]    Nearby search (default 3000m)
  detail    <id>                              POI detail by Amap ID
  geo       <address>                         Geocode address to coordinates
  regeo     <location>                        Reverse geocode coordinates
  driving   <origin> <destination>            Driving route (auto geocode)
  walking   <origin> <destination>            Walking route (auto geocode)
  bicycling <origin> <destination>            Bicycling route (auto geocode)
  transit   <origin> <destination> <city> <cityd>
                                              Transit route (city names required)
  distance  <origins> <destination> [type]    Distance measurement (1 driving / 0 straight / 3 walking)
  ip        [ip]                              IP location`),
	)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}

	cmd.AddCommand(
		c.cmdSearch().command(),
		c.cmdAround().command(),
		c.cmdDetail().command(),
		c.cmdGeo().command(),
		c.cmdRegeo().command(),
		c.cmdDriving().command(),
		c.cmdWalking().command(),
		c.cmdBicycling().command(),
		c.cmdTransit().command(),
		c.cmdDistance().command(),
		c.cmdIP().command(),
	)
	return cmd
}

// --- 子命令工厂 ---

func (c *cmdAmap) cmdSearch() *cmdAmapSub {
	return &cmdAmapSub{
		global: c.global,
		name:   "search",
		usage:  amapSearchUsage,
		short:  i18n.G("Search POIs by keywords"),
		long:   i18n.G("POI keyword search, restricted to the given city when provided (citylimit=true)."),
		action: c.doSearch,
	}
}

func (c *cmdAmap) cmdAround() *cmdAmapSub {
	return &cmdAmapSub{
		global: c.global,
		name:   "around",
		usage:  amapAroundUsage,
		short:  i18n.G("Search POIs around a location"),
		long:   i18n.G("Nearby POI search around a coordinate point (经度,纬度); default radius 3000m."),
		action: c.doAround,
	}
}

func (c *cmdAmap) cmdDetail() *cmdAmapSub {
	return &cmdAmapSub{
		global: c.global,
		name:   "detail",
		usage:  amapDetailUsage,
		short:  i18n.G("Show POI detail by ID"),
		long:   i18n.G("Fetch detailed information of a POI by its Amap POI ID."),
		action: c.doDetail,
	}
}

func (c *cmdAmap) cmdGeo() *cmdAmapSub {
	return &cmdAmapSub{
		global: c.global,
		name:   "geo",
		usage:  amapGeoUsage,
		short:  i18n.G("Geocode an address"),
		long:   i18n.G("Convert an address to GCJ-02 coordinates (经度,纬度)."),
		action: c.doGeo,
	}
}

func (c *cmdAmap) cmdRegeo() *cmdAmapSub {
	return &cmdAmapSub{
		global: c.global,
		name:   "regeo",
		usage:  amapRegeoUsage,
		short:  i18n.G("Reverse geocode coordinates"),
		long:   i18n.G("Convert GCJ-02 coordinates (经度,纬度) to an address."),
		action: c.doRegeo,
	}
}

func (c *cmdAmap) cmdDriving() *cmdAmapSub {
	return &cmdAmapSub{
		global: c.global,
		name:   "driving",
		usage:  amapRouteUsage,
		short:  i18n.G("Plan a driving route"),
		long:   i18n.G("Plan a driving route between two addresses (both are geocoded automatically)."),
		action: c.doDriving,
	}
}

func (c *cmdAmap) cmdWalking() *cmdAmapSub {
	return &cmdAmapSub{
		global: c.global,
		name:   "walking",
		usage:  amapRouteUsage,
		short:  i18n.G("Plan a walking route"),
		long:   i18n.G("Plan a walking route between two addresses (both are geocoded automatically)."),
		action: c.doWalking,
	}
}

func (c *cmdAmap) cmdBicycling() *cmdAmapSub {
	return &cmdAmapSub{
		global: c.global,
		name:   "bicycling",
		usage:  amapRouteUsage,
		short:  i18n.G("Plan a bicycling route"),
		long:   i18n.G("Plan a bicycling route between two addresses (both are geocoded automatically)."),
		action: c.doBicycling,
	}
}

func (c *cmdAmap) cmdTransit() *cmdAmapSub {
	return &cmdAmapSub{
		global: c.global,
		name:   "transit",
		usage:  amapTransitUsage,
		short:  i18n.G("Plan a transit route"),
		long:   i18n.G("Plan a public transit route; city and cityd are the origin/destination city names (required for cross-city)."),
		action: c.doTransit,
	}
}

func (c *cmdAmap) cmdDistance() *cmdAmapSub {
	return &cmdAmapSub{
		global: c.global,
		name:   "distance",
		usage:  amapDistanceUsage,
		short:  i18n.G("Measure distance"),
		long:   i18n.G("Measure distance(s): type 1 = driving, 0 = straight line, 3 = walking (default 0)."),
		action: c.doDistance,
	}
}

func (c *cmdAmap) cmdIP() *cmdAmapSub {
	return &cmdAmapSub{
		global: c.global,
		name:   "ip",
		usage:  amapIPUsage,
		short:  i18n.G("Locate an IP address"),
		long:   i18n.G("Locate an IP address; omit the argument to use the requester's IP."),
		action: c.doIP,
	}
}

// --- 动作 ---

func (c *cmdAmap) doSearch(parsed []*u.Parsed, cmd *cobra.Command) error {
	kw, err := argString(parsed, 0)
	if err != nil {
		return err
	}
	args := map[string]any{"keywords": kw, "citylimit": true}
	if city, ok := argOptionalString(parsed, 1); ok && city != "" {
		args["city"] = city
	}
	return amapCall(cmd, amap.ToolTextSearch, args)
}

func (c *cmdAmap) doAround(parsed []*u.Parsed, cmd *cobra.Command) error {
	kw, err := argString(parsed, 0)
	if err != nil {
		return err
	}
	loc, err := argString(parsed, 1)
	if err != nil {
		return err
	}
	radius := "3000"
	if s, ok := argOptionalString(parsed, 2); ok && s != "" {
		radius = s
	}
	return amapCall(cmd, amap.ToolAroundSearch, map[string]any{
		"keywords": kw, "location": loc, "radius": radius,
	})
}

func (c *cmdAmap) doDetail(parsed []*u.Parsed, cmd *cobra.Command) error {
	id, err := argString(parsed, 0)
	if err != nil {
		return err
	}
	return amapCall(cmd, amap.ToolSearchDetail, map[string]any{"id": id})
}

func (c *cmdAmap) doGeo(parsed []*u.Parsed, cmd *cobra.Command) error {
	addr, err := argString(parsed, 0)
	if err != nil {
		return err
	}
	return amapCall(cmd, amap.ToolGeo, map[string]any{"address": addr})
}

func (c *cmdAmap) doRegeo(parsed []*u.Parsed, cmd *cobra.Command) error {
	loc, err := argString(parsed, 0)
	if err != nil {
		return err
	}
	return amapCall(cmd, amap.ToolRegeocode, map[string]any{"location": loc})
}

func (c *cmdAmap) doDriving(parsed []*u.Parsed, cmd *cobra.Command) error {
	return c.doDirection(cmd, amap.ToolDirectionDriving, parsed, nil)
}

func (c *cmdAmap) doWalking(parsed []*u.Parsed, cmd *cobra.Command) error {
	return c.doDirection(cmd, amap.ToolDirectionWalking, parsed, nil)
}

func (c *cmdAmap) doBicycling(parsed []*u.Parsed, cmd *cobra.Command) error {
	return c.doDirection(cmd, amap.ToolDirectionBicycling, parsed, nil)
}

func (c *cmdAmap) doTransit(parsed []*u.Parsed, cmd *cobra.Command) error {
	city, err := argString(parsed, 2)
	if err != nil {
		return err
	}
	cityd, err := argString(parsed, 3)
	if err != nil {
		return err
	}
	return c.doDirection(cmd, amap.ToolDirectionTransit, parsed, map[string]any{
		"city": city, "cityd": cityd,
	})
}

// doDirection handles driving/walking/bicycling/transit: geocode both
// endpoints first (transit also passes city/cityd), then call the tool.
func (c *cmdAmap) doDirection(cmd *cobra.Command, tool string, parsed []*u.Parsed, extra map[string]any) error {
	origin, err := argString(parsed, 0)
	if err != nil {
		return err
	}
	dest, err := argString(parsed, 1)
	if err != nil {
		return err
	}

	client, err := newAmapClient()
	if err != nil {
		return err
	}
	o, err := geolocate(cmd.Context(), client, origin)
	if err != nil {
		return err
	}
	d, err := geolocate(cmd.Context(), client, dest)
	if err != nil {
		return err
	}
	args := map[string]any{"origin": o, "destination": d}
	for k, v := range extra {
		args[k] = v
	}
	v, err := client.Call(cmd.Context(), tool, args)
	if err != nil {
		return fmt.Errorf("%s: %w", tool, err)
	}
	return printResult(cmd, v)
}

func (c *cmdAmap) doDistance(parsed []*u.Parsed, cmd *cobra.Command) error {
	origins, err := argString(parsed, 0)
	if err != nil {
		return err
	}
	dest, err := argString(parsed, 1)
	if err != nil {
		return err
	}
	typ := "0"
	if s, ok := argOptionalString(parsed, 2); ok && s != "" {
		typ = s
	}
	return amapCall(cmd, amap.ToolDistance, map[string]any{
		"origins": origins, "destination": dest, "type": typ,
	})
}

func (c *cmdAmap) doIP(parsed []*u.Parsed, cmd *cobra.Command) error {
	args := map[string]any{}
	if ip, ok := argOptionalString(parsed, 0); ok && ip != "" {
		args["ip"] = ip
	}
	return amapCall(cmd, amap.ToolIPLocation, args)
}

// --- 辅助 ---

// amapCall resolves the key, calls the tool and prints the payload.
func amapCall(cmd *cobra.Command, tool string, args map[string]any) error {
	client, err := newAmapClient()
	if err != nil {
		return err
	}
	v, err := client.Call(cmd.Context(), tool, args)
	if err != nil {
		return fmt.Errorf("%s: %w", tool, err)
	}
	return printResult(cmd, v)
}

// newAmapClient builds a client from env AMAP_KEY or config amap.key.
func newAmapClient() (*amap.Client, error) {
	cfg, _, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	key, err := amap.ResolveKey(cfg)
	if err != nil {
		return nil, err
	}
	return amap.NewClient(key), nil
}

// geolocate maps an address to "经度,纬度" via maps_geo.
func geolocate(ctx context.Context, client *amap.Client, addr string) (string, error) {
	v, err := client.Call(ctx, amap.ToolGeo, map[string]any{"address": addr})
	if err != nil {
		return "", fmt.Errorf("geocoding %q: %w", addr, err)
	}
	loc, ok := amap.LocationFromGeo(v)
	if !ok {
		return "", fmt.Errorf("failed to geocode address: %s", addr)
	}
	return loc, nil
}

// printResult pretty-prints the tool payload (plain text passes through).
func printResult(cmd *cobra.Command, v any) error {
	if s, ok := v.(string); ok {
		fmt.Fprintln(cmd.OutOrStdout(), s)
		return nil
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling result: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}

// argString returns the i-th required argument.
func argString(parsed []*u.Parsed, i int) (string, error) {
	if i >= len(parsed) || parsed[i].Skipped {
		return "", errors.New(i18n.G("expected an argument"))
	}
	return parsed[i].String, nil
}

// argOptionalString returns the i-th optional argument if present.
func argOptionalString(parsed []*u.Parsed, i int) (string, bool) {
	if i >= len(parsed) || parsed[i].Skipped {
		return "", false
	}
	return parsed[i].String, true
}

// --- cmdAmapSub ---

// cmdAmapSub 是 amap 的子命令模板。
type cmdAmapSub struct {
	global *cmdGlobal
	name   string
	usage  u.Usage
	short  string
	long   string
	action func(parsed []*u.Parsed, cmd *cobra.Command) error
}

func (s *cmdAmapSub) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = s.name
	cmd.Short = s.short
	cmd.Long = cli.FormatSection(
		color.CyanString("Description:"),
		s.long,
	)
	cmd.RunE = s.run
	cmd.Args = cobra.ArbitraryArgs
	return cmd
}

func (s *cmdAmapSub) run(cmd *cobra.Command, args []string) error {
	parsed, err := s.global.Parse(s.usage, cmd, args)
	if err != nil {
		return err
	}
	return s.action(parsed, cmd)
}
