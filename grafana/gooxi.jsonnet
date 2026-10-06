// Grafana dashboard: Gooxi BMC (gooxi-exporter metrics)
// Style aligned with GPUStack dashboards
//   (row headers h=2, summary stats h=3, timeseries h=8 w=12)
// Generate: npm run dashboards
// Metrics: collector.go → gooxi_*

local ds = { name: '${DS_PROMETHEUS}' };

// All gooxi_* scrape metrics carry a host label with the target BMC address.
local hostFilter = 'host=~"$host"';
// Sensor panels additionally filter by sensor name.
local sensorFilter = 'name=~"$sensor"';

local defaultDatasourceText = 'Prometheus';
local defaultDatasourceUid = 'prometheus';

local variableCurrent(text, value) = {
  text: text,
  value: value,
};

local dashboardDescription =
  'Fleet view of Gooxi BMCs scraped by gooxi-exporter: reachability, chassis power, uptime, and sensor readings.';

local promDataQuery(expr, legend='', instant=false) = {
  datasource: ds,
  group: 'prometheus',
  kind: 'DataQuery',
  spec: {
    editorMode: 'code',
    exemplar: true,
    expr: expr,
    instant: instant,
    interval: '',
    legendFormat: legend,
    range: !instant,
  },
  version: 'v0',
};

local panelQuery(expr, legend='', instant=false, refId='A') = {
  kind: 'PanelQuery',
  spec: {
    hidden: false,
    query: promDataQuery(expr, legend, instant),
    refId: refId,
  },
};

local queryGroup(queries) = {
  kind: 'QueryGroup',
  spec: {
    queries: queries,
    queryOptions: {},
    transformations: [],
  },
};

local statOptions = {
  colorMode: 'value',
  graphMode: 'area',
  justifyMode: 'auto',
  orientation: 'horizontal',
  percentChangeColorMode: 'standard',
  reduceOptions: {
    calcs: ['lastNotNull'],
    fields: '',
    values: false,
  },
  showPercentChange: false,
  textMode: 'auto',
  wideLayout: true,
};

local statViz(defaults) = {
  group: 'stat',
  kind: 'VizConfig',
  spec: {
    fieldConfig: {
      defaults: defaults {
        color: { mode: 'thresholds' },
        mappings: [],
      },
      overrides: [],
    },
    options: statOptions,
  },
  version: '13.1.0',
};

local timeseriesCustom = {
  axisBorderShow: false,
  axisCenteredZero: false,
  axisColorMode: 'text',
  axisLabel: '',
  axisPlacement: 'auto',
  barAlignment: 0,
  barWidthFactor: 0.6,
  drawStyle: 'line',
  fillOpacity: 0,
  gradientMode: 'none',
  hideFrom: {
    legend: false,
    tooltip: false,
    viz: false,
  },
  insertNulls: false,
  lineInterpolation: 'linear',
  lineWidth: 1,
  pointSize: 5,
  scaleDistribution: { type: 'linear' },
  showPoints: 'never',
  showValues: false,
  spanNulls: false,
  stacking: { group: 'A', mode: 'none' },
  thresholdsStyle: { mode: 'off' },
};

local timeseriesStackedCustom = timeseriesCustom {
  fillOpacity: 20,
  stacking: { group: 'A', mode: 'normal' },
};

// Grafana timeseries axis bounds come from fieldConfig min/max.
// Nested ifs: chaining two `if` statements as `+` operands is a jsonnet
// parser bug (one branch evaluates to null).
local axisBounds(min=null, max=null) =
  if min != null then
    if max != null then { min: min, max: max }
    else { min: min }
  else
    if max != null then { max: max }
    else {};

local timeseriesViz(
  unit='short',
  stacked=false,
  min=null,
  max=null,
  steps=null,
  thresholdsStyle=null,
  fillOpacity=null,
  gradientMode=null,
  stepped=false,
  lineWidth=null,
  bars=false,
  barWidthFactor=null,
  logScale=false,
  hideZeros=false,
  tooltipMode=null,
  tooltipSort='desc',
  legendPlacement='bottom',
  legendCalcs=['last', 'max'],
  legendLimit=null,
  legendSortBy=null,
  legendSortDesc=false,
  overrides=[],
  smoothing=false,
) = {
  local baseCustom = if stacked then timeseriesStackedCustom else timeseriesCustom,
  local custom =
    (
      if thresholdsStyle != null then baseCustom { thresholdsStyle: thresholdsStyle } else baseCustom
    )
    + (if fillOpacity != null then { fillOpacity: fillOpacity } else {})
    + (if gradientMode != null then { gradientMode: gradientMode } else {})
    + (if stepped then { lineInterpolation: 'stepAfter' } else {})
    + (if smoothing then { lineInterpolation: 'smooth' } else {})
    + (if lineWidth != null then { lineWidth: lineWidth } else {})
    + (if bars then { drawStyle: 'bar' } else {})
    + (if barWidthFactor != null then { barWidthFactor: barWidthFactor } else {})
    + (if logScale then { scaleDistribution: { type: 'log' } } else {}),
  group: 'timeseries',
  kind: 'VizConfig',
  spec: {
    fieldConfig: {
      defaults: {
        color: { mode: 'palette-classic' },
        custom: custom,
        mappings: [],
        thresholds: {
          mode: 'absolute',
          steps: if steps != null then steps else [
            { color: 'green', value: 0 },
            { color: 'red', value: 80 },
          ],
        },
        unit: unit,
      } + axisBounds(min, max),
      overrides: overrides,
    },
    options: {
      legend: {
        calcs: legendCalcs,
        displayMode: 'table',
        enableFacetedFilter: false,
        overflow: 'ellipsis',
        placement: legendPlacement,
        showLegend: true,
      }
      + (if legendLimit != null then { limit: legendLimit } else {})
      + (if legendSortBy != null then { sortBy: legendSortBy, sortDesc: legendSortDesc } else {}),
      tooltip: {
        hideZeros: hideZeros,
        mode: if tooltipMode != null then tooltipMode else (if stacked then 'multi' else 'single'),
        sort: tooltipSort,
      },
    },
  },
  version: '13.1.0',
};

local bargaugeViz(
  unit='short',
  steps=[{ color: 'green', value: 0 }, { color: 'red', value: 80 }],
  overrides=[],
  limit=24,
  valueMode='color',
) = {
  group: 'bargauge',
  kind: 'VizConfig',
  spec: {
    fieldConfig: {
      defaults: {
        color: { mode: 'continuous-GrYlRd' },
        mappings: [],
        thresholds: {
          mode: 'absolute',
          steps: steps,
        },
        unit: unit,
      },
      overrides: overrides,
    },
    options: {
      displayMode: 'gradient',
      limit: limit,
      minVizHeight: 10,
      minVizWidth: 0,
      orientation: 'horizontal',
      reduceOptions: {
        calcs: ['lastNotNull'],
        fields: '',
        values: false,
      },
      showUnfilled: true,
      valueMode: valueMode,
    },
  },
  version: '13.1.0',
};

local mkPanel(id, title, description, data, vizConfig) = {
  ['panel-' + std.toString(id)]: {
    kind: 'Panel',
    spec: {
      data: data,
      description: description,
      id: id,
      links: [],
      title: title,
      vizConfig: vizConfig,
    },
  },
};

local statPanel(id, title, description, expr, fieldConfig) =
  mkPanel(
    id,
    title,
    description,
    queryGroup([panelQuery(expr, '', true)]),
    statViz(fieldConfig),
  );

// Section header — analogous to type=row in legacy GPUStack dashboards
local rowPanel(id, title) = mkPanel(
  id,
  '',
  '',
  queryGroup([]),
  {
    group: 'text',
    kind: 'VizConfig',
    spec: {
      fieldConfig: {
        defaults: {},
        overrides: [],
      },
      options: {
        code: {
          language: 'plaintext',
          showLineNumbers: false,
          showMiniMap: false,
        },
        content: '### ' + title,
        mode: 'markdown',
      },
    },
    version: '13.1.0',
  },
);

local layoutItem(id, x, y, w, h) = {
  kind: 'GridLayoutItem',
  spec: {
    element: {
      kind: 'ElementReference',
      name: 'panel-' + std.toString(id),
    },
    height: h,
    width: w,
    x: x,
    y: y,
  },
};

local defaultField = {
  thresholds: {
    mode: 'absolute',
    steps: [{ color: 'green', value: 0 }],
  },
  unit: 'none',
};

// ---------------------------------------------------------------------------
// Row headers
// ---------------------------------------------------------------------------

local summaryRow = rowPanel(100, 'Summary');
local healthRow = rowPanel(101, 'Health');
local sensorsRow = rowPanel(102, 'Sensors');
local statesRow = rowPanel(103, 'Sensor States');

// ---------------------------------------------------------------------------
// Summary KPIs (compact stats)
// ---------------------------------------------------------------------------

local bmcOnlinePanel = statPanel(
  1,
  'BMCs Online',
  'Number of BMCs whose last scrape succeeded (sum of gooxi_up).',
  'sum(gooxi_up{' + hostFilter + '}) or vector(0)',
  defaultField,
);

local bmcOfflinePanel = statPanel(
  2,
  'BMCs Offline',
  'BMCs whose last scrape failed (login error, timeout, unreachable).',
  'count(gooxi_up{' + hostFilter + '} == 0) or vector(0)',
  {
    unit: 'none',
    thresholds: {
      mode: 'absolute',
      steps: [
        { color: 'green', value: 0 },
        { color: 'red', value: 1 },
      ],
    },
  },
);

local chassisPowerPanel = statPanel(
  3,
  'Chassis Power On',
  'Number of BMCs reporting chassis power on.',
  'sum(gooxi_chassis_power_on{' + hostFilter + '}) or vector(0)',
  defaultField,
);

local sensorWarningsPanel = statPanel(
  4,
  'Sensor Warnings',
  'Sensors currently in warning state (sensor_state = 2).',
  'sum(gooxi_sensor_state{' + hostFilter + ',' + sensorFilter + '} == 2) or vector(0)',
  {
    unit: 'none',
    thresholds: {
      mode: 'absolute',
      steps: [
        { color: 'green', value: 0 },
        { color: '#EAB839', value: 1 },
      ],
    },
  },
);

local sensorCriticalPanel = statPanel(
  5,
  'Sensor Critical',
  'Sensors currently in critical state (sensor_state = 3).',
  'sum(gooxi_sensor_state{' + hostFilter + ',' + sensorFilter + '} == 3) or vector(0)',
  {
    unit: 'none',
    thresholds: {
      mode: 'absolute',
      steps: [
        { color: 'green', value: 0 },
        { color: 'red', value: 1 },
      ],
    },
  },
);

local avgScrapeDurationPanel = statPanel(
  6,
  'Avg Scrape Duration',
  'Average time the exporter spends on one BMC scrape (login + API calls + logout).',
  'avg(gooxi_scrape_duration_seconds{' + hostFilter + '}) or vector(0)',
  {
    unit: 's',
    thresholds: {
      mode: 'absolute',
      steps: [
        { color: 'green', value: 0 },
        { color: '#EAB839', value: 5 },
        { color: 'red', value: 15 },
      ],
    },
  },
);

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

local reachabilityPanel = mkPanel(
  10,
  'BMC Reachability',
  'Last scrape result per BMC: 1 = OK, 0 = login/API failure. Dips to 0 are outages or flapping sessions.',
  queryGroup([
    panelQuery('gooxi_up{' + hostFilter + '}', '{{host}}', false, 'A'),
  ]),
  timeseriesViz(
    'none',
    min=0,
    max=1,
    steps=[
      { color: 'red', value: 0 },
      { color: 'green', value: 1 },
    ],
    thresholdsStyle={ mode: 'line' },
    stepped=true,
  ),
);

local scrapeDurationPanel = mkPanel(
  11,
  'Scrape Duration',
  'Wall time of one full BMC scrape per host. Sustained growth points at a slow BMC or network path.',
  queryGroup([
    panelQuery('gooxi_scrape_duration_seconds{' + hostFilter + '}', '{{host}}', false, 'A'),
  ]),
  timeseriesViz('s', min=0),
);

local chassisPowerOverTimePanel = mkPanel(
  12,
  'Chassis Power State',
  'Chassis power per BMC over time (1 = on, 0 = off). Unplanned drops to 0 are power events.',
  queryGroup([
    panelQuery('gooxi_chassis_power_on{' + hostFilter + '}', '{{host}}', false, 'A'),
  ]),
  timeseriesViz(
    'none',
    min=0,
    max=1,
    steps=[
      { color: 'red', value: 0 },
      { color: 'green', value: 1 },
    ],
    thresholdsStyle={ mode: 'line' },
    stepped=true,
  ),
);

local uptimePanel = mkPanel(
  13,
  'Uptime',
  'System uptime from the BMC POH counter. A reset to zero means the machine was powered cycled.',
  queryGroup([
    panelQuery('gooxi_uptime_seconds{' + hostFilter + '}', '{{host}}', false, 'A'),
  ]),
  timeseriesViz('s', min=0),
);

// ---------------------------------------------------------------------------
// Sensors (one panel per sensor type)
// ---------------------------------------------------------------------------

local sensorPanel(id, title, description, typeExpr, unit) =
  mkPanel(
    id,
    title,
    description,
    queryGroup([
      panelQuery(
        'gooxi_sensor_value{' + hostFilter + ',' + sensorFilter + ',type=' + typeExpr + '}',
        '{{host}} / {{name}}',
        false,
        'A',
      ),
    ]),
    timeseriesViz(unit, min=0, legendSortBy='name', legendCalcs=['last', 'max']),
  );

local temperaturePanel = sensorPanel(
  20,
  'Temperature',
  'All temperature sensors (CPU, memory, motherboard, inlet) in degrees Celsius.',
  '"temperature"',
  'degC',
);

local voltagePanel = sensorPanel(
  21,
  'Voltage',
  'Rail voltages reported by the BMC. Drift outside the rail window shows up here first.',
  '"voltage"',
  'volt',
);

local fanPanel = sensorPanel(
  22,
  'Fan Speed',
  'Cooling fan speeds in RPM. A flat-zero line means a stopped fan; spikes mean thermal load.',
  '"fan"',
  'rpm',
);

local powerSupplyPanel = sensorPanel(
  23,
  'Power Supply',
  'Power supply and power unit readings (input/output power) in watts.',
  '~"power_supply|power_unit"',
  'watt',
);

local currentPanel = sensorPanel(
  24,
  'Current',
  'Current draw reported by the BMC in amperes.',
  '"current"',
  'amp',
);

local processorPanel = sensorPanel(
  25,
  'Processor',
  'Processor utilization and related CPU sensors reported by the BMC.',
  '"processor"',
  'percent',
);

local coolingDevicePanel = sensorPanel(
  26,
  'Cooling Device',
  'Cooling device sensors (non-fan) reported by the BMC.',
  '"cooling_device"',
  'short',
);

local physicalSecurityPanel = sensorPanel(
  27,
  'Physical Security',
  'Physical security intrusion sensors (e.g. chassis intrusion flag). Non-zero means an event.',
  '"physical_security"',
  'none',
);

// ---------------------------------------------------------------------------
// Sensor States
// ---------------------------------------------------------------------------

local nonNormalSensorsPanel = mkPanel(
  30,
  'Non-Normal Sensors',
  'Sensors with sensor_state > 1 (2 = warning, 3 = critical). Only alerting series are shown.',
  queryGroup([
    panelQuery(
      'gooxi_sensor_state{' + hostFilter + ',' + sensorFilter + '} > 1',
      '{{host}} / {{name}}',
      false,
      'A',
    ),
  ]),
  timeseriesViz(
    'none',
    true,
    min=0,
    max=3,
    steps=[
      { color: 'green', value: 0 },
      { color: '#EAB839', value: 2 },
      { color: 'red', value: 3 },
    ],
    bars=true,
    barWidthFactor=0.7,
    hideZeros=true,
    legendCalcs=['last', 'max'],
  ),
);

local worstStatePerHostPanel = mkPanel(
  31,
  'Worst Sensor State per Host',
  'Max sensor state per BMC: 1 = all normal, 2 = has warnings, 3 = has criticals.',
  queryGroup([
    panelQuery(
      'max by (host) (gooxi_sensor_state{' + hostFilter + ',' + sensorFilter + '})',
      '{{host}}',
      true,
      'A',
    ),
  ]),
  bargaugeViz(
    'none',
    [
      { color: 'green', value: 1 },
      { color: '#EAB839', value: 2 },
      { color: 'red', value: 3 },
    ],
  ),
);

// ---------------------------------------------------------------------------
// Panels aggregation
// ---------------------------------------------------------------------------

local panels =
  summaryRow
  + healthRow
  + sensorsRow
  + statesRow
  // Summary
  + bmcOnlinePanel
  + bmcOfflinePanel
  + chassisPowerPanel
  + sensorWarningsPanel
  + sensorCriticalPanel
  + avgScrapeDurationPanel
  // Health
  + reachabilityPanel
  + scrapeDurationPanel
  + chassisPowerOverTimePanel
  + uptimePanel
  // Sensors
  + temperaturePanel
  + voltagePanel
  + fanPanel
  + powerSupplyPanel
  + currentPanel
  + processorPanel
  + coolingDevicePanel
  + physicalSecurityPanel
  // Sensor States
  + nonNormalSensorsPanel
  + worstStatePerHostPanel;

{
  annotations: [
    {
      kind: 'AnnotationQuery',
      spec: {
        builtIn: true,
        enable: true,
        hide: true,
        iconColor: 'rgba(0, 211, 255, 1)',
        name: 'Annotations & Alerts',
        query: {
          datasource: { name: 'grafana' },
          group: 'grafana',
          kind: 'DataQuery',
          spec: {
            limit: 100,
            matchAny: false,
            tags: [],
            type: 'dashboard',
          },
          version: 'v0',
        },
      },
    },
  ],
  cursorSync: 'Off',
  description: dashboardDescription,
  editable: true,
  elements: panels,
  layout: {
    kind: 'GridLayout',
    spec: {
      // Layout mirrors GPUStack Model:
      //   row h=2 → summary stats h=3 → section pairs of h=8 w=12
      items: [
        // === Summary ===
        layoutItem(100, 0, 0, 24, 2),
        layoutItem(1, 0, 2, 4, 3),     // BMCs Online
        layoutItem(2, 4, 2, 4, 3),     // BMCs Offline
        layoutItem(3, 8, 2, 4, 3),     // Chassis Power On
        layoutItem(4, 12, 2, 4, 3),    // Sensor Warnings
        layoutItem(5, 16, 2, 4, 3),    // Sensor Critical
        layoutItem(6, 20, 2, 4, 3),    // Avg Scrape Duration

        // === Health ===
        layoutItem(101, 0, 5, 24, 2),
        layoutItem(10, 0, 7, 12, 8),   // BMC Reachability
        layoutItem(11, 12, 7, 12, 8),  // Scrape Duration
        layoutItem(12, 0, 15, 12, 8),  // Chassis Power State
        layoutItem(13, 12, 15, 12, 8), // Uptime

        // === Sensors ===
        layoutItem(102, 0, 23, 24, 2),
        layoutItem(20, 0, 25, 12, 8),  // Temperature
        layoutItem(21, 12, 25, 12, 8), // Voltage
        layoutItem(22, 0, 33, 12, 8),  // Fan Speed
        layoutItem(23, 12, 33, 12, 8), // Power Supply
        layoutItem(24, 0, 41, 12, 8),  // Current
        layoutItem(25, 12, 41, 12, 8), // Processor
        layoutItem(26, 0, 49, 12, 8),  // Cooling Device
        layoutItem(27, 12, 49, 12, 8), // Physical Security

        // === Sensor States ===
        layoutItem(103, 0, 57, 24, 2),
        layoutItem(30, 0, 59, 12, 8),  // Non-Normal Sensors
        layoutItem(31, 12, 59, 12, 8), // Worst Sensor State per Host
      ],
    },
  },
  links: [],
  liveNow: false,
  preload: false,
  tags: ['gooxi', 'bmc'],
  timeSettings: {
    autoRefresh: 'auto',
    autoRefreshIntervals: ['5s', '10s', '30s', '1m', '5m', '15m', '30m', '1h', '2h', '1d'],
    fiscalYearStartMonth: 0,
    from: 'now-6h',
    hideTimepicker: false,
    timezone: 'browser',
    to: 'now',
  },
  title: 'Gooxi BMC',
  uid: 'gooxi-bmc',
  variables: [
    {
      kind: 'QueryVariable',
      spec: {
        allValue: '.*',
        allowCustomValue: true,
        current: variableCurrent('All', '$__all'),
        definition: 'label_values(gooxi_up, host)',
        hide: 'dontHide',
        includeAll: true,
        label: 'Host',
        multi: true,
        name: 'host',
        options: [],
        query: {
          datasource: ds,
          group: 'prometheus',
          kind: 'DataQuery',
          spec: {
            qryType: 1,
            query: 'label_values(gooxi_up, host)',
            refId: 'PrometheusVariableQueryEditor-VariableQuery',
          },
          version: 'v0',
        },
        refresh: 'onDashboardLoad',
        regex: '',
        regexApplyTo: 'value',
        skipUrlSync: false,
        sort: 'alphabeticalAsc',
      },
    },
    {
      kind: 'QueryVariable',
      spec: {
        allValue: '.*',
        allowCustomValue: true,
        current: variableCurrent('All', '$__all'),
        definition: 'label_values(gooxi_sensor_value, name)',
        hide: 'dontHide',
        includeAll: true,
        label: 'Sensor',
        multi: true,
        name: 'sensor',
        options: [],
        query: {
          datasource: ds,
          group: 'prometheus',
          kind: 'DataQuery',
          spec: {
            qryType: 1,
            query: 'label_values(gooxi_sensor_value, name)',
            refId: 'PrometheusVariableQueryEditor-VariableQuery',
          },
          version: 'v0',
        },
        refresh: 'onDashboardLoad',
        regex: '',
        regexApplyTo: 'value',
        skipUrlSync: false,
        sort: 'alphabeticalAsc',
      },
    },
    {
      kind: 'DatasourceVariable',
      spec: {
        allowCustomValue: true,
        current: variableCurrent(defaultDatasourceText, defaultDatasourceUid),
        hide: 'dontHide',
        includeAll: false,
        label: 'Datasource',
        multi: false,
        name: 'DS_PROMETHEUS',
        options: [],
        pluginId: 'prometheus',
        refresh: 'onDashboardLoad',
        regex: '',
        skipUrlSync: false,
      },
    },
  ],
  version: std.parseInt(std.extVar('dashboard_version')),
}
