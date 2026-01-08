
// ECharts implementation for Wakapi Summary Page

const LEGEND_CHARACTERS = 20

const baseColors = [
    '#112836',
    '#163B43',
    '#1C4F4D',
    '#215B4C',
    '#276749',
    '#437C57',
    '#5F9167',
    '#7DA67C',
    '#9FBA98',
    '#BFCEB5',
    '#DCE2D3'
];

// Category Translation Map
const categoryTranslations = {
    'coding': '编码',
    'browsing': '浏览',
    'building': '构建',
    'debugging': '调试',
    'designing': '设计',
    'writing tests': '编写测试',
    'writing docs': '编写文档',
    'code reviewing': '代码审查',
    'communicating': '沟通',
    'researching': '研究',
    'learning': '学习',
    'unknown': '其他',
};

function translateCategory(category) {
    return categoryTranslations[category] || category;
}

const charts = {}; // Store chart instances
const chartIds = [
    'chart-projects', 'chart-os', 'chart-editor', 'chart-language',
    'chart-machine', 'chart-label', 'chart-branches', 'chart-entities',
    'chart-categories', 'chart-timeline', 'chart-hourly'
];
const containerIds = [
    'project-container', 'os-container', 'editor-container', 'language-container',
    'machine-container', 'label-container', 'branch-container', 'entity-container',
    'category-container', 'timeline-container', 'hourly-container'
];

// Data indices matching the order in summary.js
const DATA_INDICES = {
    projects: 0,
    os: 1,
    editors: 2,
    languages: 3,
    machines: 4,
    labels: 5,
    branches: 6,
    entities: 7,
    categories: 8,
    timeline: 9,
    hourly: 10
};

const dataSources = [
    wakapiData.projects, wakapiData.operatingSystems, wakapiData.editors,
    wakapiData.languages, wakapiData.machines, wakapiData.labels,
    wakapiData.branches, wakapiData.entities, wakapiData.categories,
    wakapiData.timelineStats, wakapiData.hourlyBreakdown
];

let showTopN = [];

// Helper prototypes
String.prototype.toHHMMSS = function () {
    const sec_num = parseInt(this, 10)
    let hours = Math.floor(sec_num / 3600)
    let minutes = Math.floor((sec_num - (hours * 3600)) / 60)
    let seconds = sec_num - (hours * 3600) - (minutes * 60)

    if (hours < 10) { hours = '0' + hours }
    if (minutes < 10) { minutes = '0' + minutes }
    if (seconds < 10) { seconds = '0' + seconds }
    return `${hours}:${minutes}:${seconds}`
}

function getRandomColor(seed) {
    seed = seed ? seed : '1234567'
    Math.seedrandom(seed)
    var letters = '0123456789ABCDEF'.split('')
    var color = '#'
    for (var i = 0; i < 6; i++) {
        color += letters[Math.floor(Math.random() * 16)]
    }
    return color
}

function getColor(seed, index) {
    if (index < baseColors.length) return baseColors[(index + 5) % baseColors.length]
    return getRandomColor(seed)
}

function formatDuration(seconds) {
    return seconds.toString().toHHMMSS();
}

function initChart(domId) {
    const dom = document.getElementById(domId);
    if (!dom) return null;
    // ECharts needs a container with specific size.
    // We assume CSS/Tailwind provides the size to the container div.
    const chart = echarts.init(dom, 'dark', {
        renderer: 'canvas',
        useDirtyRect: false
    });
    return chart;
}

function getCommonOption(title, tooltipFormatter) {
    return {
        backgroundColor: 'transparent',
        textStyle: {
            fontFamily: 'Source Sans 3, Roboto, Helvetica Neue, Arial, sens-serif'
        },
        tooltip: {
            trigger: 'item',
            formatter: tooltipFormatter,
            backgroundColor: 'rgba(50, 50, 50, 0.9)',
            textStyle: {
                color: '#fff'
            },
            borderWidth: 0
        },
        grid: {
            left: '3%',
            right: '4%',
            bottom: '3%',
            containLabel: true
        }
    };
}

function updateCharts(subselection) {
    const vibrantColors = JSON.parse(window.localStorage.getItem('wakapi_vibrant_colors') || false);

    // --- Projects Chart (Bar) ---
    if (shouldUpdate(DATA_INDICES.projects, subselection)) {
        renderBarChart('chart-projects', wakapiData.projects, showTopN[DATA_INDICES.projects], vibrantColors, true, (params) => {
             const url = new URL(window.location.href)
             const name = params.name
             url.searchParams.set('project', name === 'unknown' ? '-' : name)
             window.location.href = url.href
        });
    }

    // --- OS Chart (Pie) ---
    if (shouldUpdate(DATA_INDICES.os, subselection)) {
        renderPieChart('chart-os', wakapiData.operatingSystems, showTopN[DATA_INDICES.os], vibrantColors, osColors);
    }

    // --- Editors Chart (Pie) ---
    if (shouldUpdate(DATA_INDICES.editors, subselection)) {
        renderPieChart('chart-editor', wakapiData.editors, showTopN[DATA_INDICES.editors], vibrantColors, editorColors);
    }

    // --- Languages Chart (Pie) ---
    if (shouldUpdate(DATA_INDICES.languages, subselection)) {
        renderPieChart('chart-language', wakapiData.languages, showTopN[DATA_INDICES.languages], vibrantColors, languageColors);
    }

    // --- Machines Chart (Pie) ---
    if (shouldUpdate(DATA_INDICES.machines, subselection)) {
        renderPieChart('chart-machine', wakapiData.machines, showTopN[DATA_INDICES.machines], vibrantColors, {});
    }

    // --- Labels Chart (Pie) ---
    if (shouldUpdate(DATA_INDICES.labels, subselection)) {
        renderPieChart('chart-label', wakapiData.labels, showTopN[DATA_INDICES.labels], vibrantColors, {});
    }

    // --- Branches Chart (Bar) ---
    if (shouldUpdate(DATA_INDICES.branches, subselection)) {
        renderBarChart('chart-branches', wakapiData.branches, showTopN[DATA_INDICES.branches], vibrantColors, false);
    }

    // --- Entities Chart (Bar) ---
    if (shouldUpdate(DATA_INDICES.entities, subselection)) {
        // Transform keys for display
        const transformedData = wakapiData.entities.map(e => ({ ...e, key: extractFile(e.key) }));
        renderBarChart('chart-entities', transformedData, showTopN[DATA_INDICES.entities], vibrantColors, false);
    }

    // --- Categories Chart (Bar) ---
    if (shouldUpdate(DATA_INDICES.categories, subselection)) {
        renderCategoryChart('chart-categories', wakapiData.categories, showTopN[DATA_INDICES.categories], vibrantColors);
    }

    // --- Timeline Chart (Stacked Bar) ---
    if (shouldUpdate(DATA_INDICES.timeline, subselection)) {
        renderTimelineChart('chart-timeline', wakapiData.timelineStats, vibrantColors);
    }

    // --- Hourly Chart (Stacked Bar) ---
    if (shouldUpdate(DATA_INDICES.hourly, subselection)) {
        renderHourlyChart('chart-hourly', wakapiData.hourlyBreakdown, vibrantColors);
    }
}

function shouldUpdate(index, subselection) {
    const dom = document.getElementById(chartIds[index]);
    if (!dom || dom.classList.contains('hidden')) return false;
    // Check if data exists for this index (some are context dependent like branches)
    if (!dataSources[index]) return false;
    
    // For simple arrays (top 8), check length against N if filtered
    // But honestly, re-rendering everything is fine usually unless performance is critical
    return !subselection || subselection.includes(index);
}

function renderBarChart(domId, data, limit, vibrantColors, clickable, clickHandler) {
    const chart = getChart(domId);
    if (!chart) return;

    const slicedData = data.slice(0, Math.min(limit, data.length));
    const categories = slicedData.map(d => d.key);
    const values = slicedData.map(d => d.total);
    const colors = slicedData.map((d, i) => vibrantColors ? getRandomColor(d.key) : getColor(d.key, i % baseColors.length));

    const option = {
        ...getCommonOption(),
        tooltip: {
            trigger: 'axis',
            axisPointer: { type: 'shadow' },
            formatter: (params) => {
                const param = params[0];
                return `${param.name}<br/>${param.marker} ${formatDuration(param.value)}`;
            }
        },
        grid: {
             left: '3%',
             right: '10%', // more space for labels
             bottom: '3%',
             top: '3%',
             containLabel: true
        },
        xAxis: {
            type: 'value',
            axisLabel: {
                formatter: (value) => formatDuration(value)
            },
            splitLine: { show: false }
        },
        yAxis: {
            type: 'category',
            data: categories,
            inverse: true, // Top items at top
            axisLabel: {
                width: 100, // limit width
                overflow: 'truncate',
                interval: 0
            }
        },
        series: [{
            data: values.map((v, i) => ({
                value: v,
                itemStyle: { color: colors[i] }
            })),
            type: 'bar',
            barWidth: '60%',
            label: {
                show: true,
                position: 'right',
                formatter: (params) => formatDuration(params.value),
                color: '#aaa'
            }
        }]
    };

    chart.setOption(option);
    
    if (clickable && clickHandler) {
        chart.off('click');
        chart.on('click', clickHandler);
    }
    
    // Cursor pointer if clickable
    if (clickable) {
        chart.getZr().on('mousemove', function (params) {
            var pointInPixel = [params.offsetX, params.offsetY];
            if (chart.containPixel('grid', pointInPixel)) {
                chart.getZr().setCursorStyle('pointer');
            } else {
                chart.getZr().setCursorStyle('default');
            }
        });
    }
}

function renderPieChart(domId, data, limit, vibrantColors, colorMap) {
    const chart = getChart(domId);
    if (!chart) return;

    const slicedData = data.slice(0, Math.min(limit, data.length));
    
    const seriesData = slicedData.map((d, i) => ({
        value: d.total,
        name: d.key,
        itemStyle: {
            color: vibrantColors 
                ? ((colorMap && colorMap[d.key.toLowerCase()]) || getRandomColor(d.key)) 
                : getColor(d.key, i)
        }
    }));

    const option = {
        ...getCommonOption(),
        tooltip: {
            trigger: 'item',
            formatter: (params) => {
                return `${params.name}<br/>${params.marker} ${formatDuration(params.value)} (${params.percent}%)`;
            }
        },
        legend: {
            type: 'scroll',
            orient: 'vertical',
            right: 0,
            top: 10,
            bottom: 10,
            textStyle: { color: '#aaa' },
            formatter: (name) => {
                return name.length > LEGEND_CHARACTERS 
                    ? name.slice(0, LEGEND_CHARACTERS - 3) + '...' 
                    : name;
            }
        },
        series: [{
            name: 'Access From',
            type: 'pie',
            radius: ['40%', '70%'],
            center: ['40%', '50%'], // Move left to make space for legend
            avoidLabelOverlap: false,
            itemStyle: {
                borderRadius: 5,
                borderColor: '#1a1b1e', // Match card background roughly
                borderWidth: 2
            },
            label: {
                show: false,
                position: 'center'
            },
            emphasis: {
                label: {
                    show: false,
                    fontSize: 20,
                    fontWeight: 'bold'
                }
            },
            labelLine: {
                show: false
            },
            data: seriesData
        }]
    };

    chart.setOption(option);
}

function renderCategoryChart(domId, data, limit, vibrantColors) {
    const chart = getChart(domId);
    if (!chart) return;

    // Categories is likely "one bar per category" or "stacked bar"?
    // The original code was: indexAxis: 'y', stacked: true.
    // It seems it was a single stacked bar showing distribution? 
    // Wait, labels: ['Categories']. So it was one thick bar stacked.
    
    // Let's replicate the single stacked bar.
    
    const slicedData = data.slice(0, Math.min(limit, data.length));
    const series = slicedData.map((d, i) => ({
        name: translateCategory(d.key),
        type: 'bar',
        stack: 'total',
        label: { show: true, formatter: '{a}' }, // Show series name inside bar
        data: [d.total],
        itemStyle: {
             color: vibrantColors ? getRandomColor(d.key) : getColor(d.key, i % baseColors.length)
        }
    }));

    const option = {
        ...getCommonOption(),
        tooltip: {
            trigger: 'item',
             formatter: (params) => {
                return `${params.seriesName}<br/>${params.marker} ${formatDuration(params.value)}`;
            }
        },
        grid: {
            top: '10%',
            left: '3%',
            right: '4%',
            bottom: '10%',
            containLabel: true
        },
        xAxis: {
            type: 'value',
            axisLabel: { formatter: (value) => formatDuration(value) },
            splitLine: { show: false }
        },
        yAxis: {
            type: 'category',
            data: ['Categories'],
            axisTick: { show: false },
            axisLine: { show: false }
        },
        series: series
    };

    chart.setOption(option);
}

function renderTimelineChart(domId, data, vibrantColors) {
    const chart = getChart(domId);
    if (!chart) return;

    const dates = data.map(day => new Date(day.date).toLocaleDateString('zh-CN'));
    
    // Identify all projects
    const allProjects = [...new Set(data.flatMap(day => day.projects.map(p => p.name)))].sort();
    
    const series = allProjects.map((project, i) => {
        const projectData = data.map(day => {
            const p = day.projects.find(dp => dp.name === project);
            return p ? p.duration : 0;
        });
        
        return {
            name: project,
            type: 'bar',
            stack: 'total',
            emphasis: { focus: 'series' },
            data: projectData,
            itemStyle: {
                color: vibrantColors ? getRandomColor(project) : getColor(project, i % baseColors.length)
            }
        };
    });

    const option = {
        ...getCommonOption(),
        tooltip: {
            trigger: 'axis',
            axisPointer: { type: 'shadow' },
            formatter: (params) => {
                let rel = params[0].axisValueLabel + '<br/>';
                params.forEach(param => {
                    if (param.value > 0) {
                        rel += `${param.marker} ${param.seriesName}: ${formatDuration(param.value)}<br/>`;
                    }
                });
                return rel;
            }
        },
        legend: {
            type: 'scroll',
            textStyle: { color: '#aaa' },
             right: 10,
             top: 0
        },
        grid: {
            left: '3%',
            right: '4%',
            bottom: '3%',
            containLabel: true
        },
        xAxis: {
            type: 'category',
            data: dates
        },
        yAxis: {
            type: 'value',
             axisLabel: { formatter: (value) => formatDuration(value) }
        },
        series: series
    };

    chart.setOption(option);
}

function renderHourlyChart(domId, data, vibrantColors) {
    const chart = getChart(domId);
    if (!chart) return;
    
    // Custom renderer for ECharts to mimic the time-range bars
    // Or we can use a custom series. 
    // The original chart used floating bars [start, end].
    // ECharts "custom" series can do this.
    
    const projects = data.map(d => d.project);
    
    // We need to flatten the data for custom series
    // Item: { projectIndex, start, end, duration, entity, color }
    const seriesData = [];
    
    data.forEach((projData, pIdx) => {
        let pre = 0; // Relative start if we wanted stacked, but here we want absolute time on X?
        // Wait, original chart x-axis was Time.
        // "min: +new Date(wakapiData.hourlyBreakdownFromTime)"
        // And data was [fromTime, toTime].
        
        projData.items.forEach((item, i) => {
            const startTime = new Date(item.from_time).getTime();
            const durationMs = item.duration / 1e6; // duration is ns? Original: duration / 1e9 * 1e3 => ms. 
            // 1 ns = 1e-6 ms. 
            // 1 s = 1e9 ns. 
            // item.duration is in ns.
            // item.duration / 1e6 = ms.
            const endTime = startTime + (item.duration / 1e6);
            
            seriesData.push({
                name: item.entity,
                value: [
                    pIdx, // Y axis category index
                    startTime, // X start
                    endTime, // X end
                    item.duration / 1e9 // Duration in seconds for tooltip
                ],
                itemStyle: {
                     color: vibrantColors ? getRandomColor(projData.project) : getColor(projData.project, pIdx % baseColors.length)
                }
            });
        });
    });

    const option = {
         ...getCommonOption(),
         tooltip: {
            formatter: (params) => {
                return `${params.name}<br/>${params.marker} ${new Date(params.value[1]).toLocaleTimeString()} - ${new Date(params.value[2]).toLocaleTimeString()}<br/>Duration: ${formatDuration(params.value[3])}`;
            }
        },
        grid: {
            left: '3%',
            right: '4%',
            bottom: '3%',
            containLabel: true
        },
        xAxis: {
            type: 'time',
            min: new Date(wakapiData.hourlyBreakdownFromTime).getTime(),
            max: new Date(wakapiData.hourlyBreakdownToTime).getTime(),
            axisLabel: {
                formatter: {
                    hour: '{HH}:{mm}',
                    minute: '{HH}:{mm}'
                }
            }
        },
        yAxis: {
            type: 'category',
            data: projects,
        },
        series: [{
            type: 'custom',
            renderItem: (params, api) => {
                const categoryIndex = api.value(0);
                const start = api.coord([api.value(1), categoryIndex]);
                const end = api.coord([api.value(2), categoryIndex]);
                const height = api.size([0, 1])[1] * 0.6; // 60% bar height
                
                const rectShape = echarts.graphic.clipRectByRect({
                    x: start[0],
                    y: start[1] - height / 2,
                    width: end[0] - start[0],
                    height: height
                }, {
                    x: params.coordSys.x,
                    y: params.coordSys.y,
                    width: params.coordSys.width,
                    height: params.coordSys.height
                });
                
                return rectShape && {
                    type: 'rect',
                    transition: ['shape'],
                    shape: rectShape,
                    style: api.style()
                };
            },
            data: seriesData,
            encode: {
                x: [1, 2],
                y: 0,
                tooltip: [1, 2],
                itemName: 3 
            }
        }],
        dataZoom: [
            {
                type: 'slider',
                filterMode: 'weakFilter',
                showDataShadow: false,
                top: 400, // Move to bottom?
                labelFormatter: ''
            },
            {
                type: 'inside',
                filterMode: 'weakFilter'
            }
        ]
    };

    chart.setOption(option);
}


function getChart(domId) {
    let chart = charts[domId];
    if (!chart) {
        chart = initChart(domId);
        if (chart) charts[domId] = chart;
    }
    return chart;
}

function extractFile(filePath) {
    const delimiter = filePath.includes('\\') ? '\\' : '/'
    return filePath.split(delimiter).at(-1)
}

function parseTopN() {
    let topNPickers = [...document.getElementsByClassName('top-picker')]
    topNPickers.sort(((a, b) => parseInt(a.attributes['data-entity'].value) - parseInt(b.attributes['data-entity'].value)))
    showTopN = topNPickers.map(e => parseInt(e.value))
}

function togglePlaceholders(mask) {
    const placeholderElements = containerIds.map(id => document.querySelector(`#${id} .placeholder-container`));
    const canvasElements = chartIds.map(id => document.getElementById(id));

    for (let i = 0; i < mask.length; i++) {
        if (!placeholderElements[i] || !canvasElements[i]) continue;
        
        if (!mask[i]) {
            canvasElements[i].classList.add('hidden')
            placeholderElements[i].classList.remove('hidden')
        } else {
            canvasElements[i].classList.remove('hidden')
            placeholderElements[i].classList.add('hidden')
        }
    }
}

function getPresentDataMask() {
    return dataSources.map(list => (list ? list.reduce((acc, e) => acc + (e.total ? e.total : ((e.projects || e.items) ? (e.projects || e.items).reduce((acc, f) => acc + f.duration, 0) : 0)), 0) : 0) > 0)
}

function updateNumTotal() {
    for (let i = 0; i < dataSources.length - 2; i++) {
        const span = document.querySelector(`span[data-entity='${i}']`);
        if (span) span.innerText = dataSources[i].length.toString();
    }
}

function swapCharts(showEntity, hideEntity) {
    document.getElementById(`${showEntity}-container`).classList.remove('hidden')
    document.getElementById(`${hideEntity}-container`).classList.add('hidden')
    
    // Trigger resize for the shown chart
    const chartId = `chart-${showEntity}`;
    if (charts[chartId]) {
        charts[chartId].resize();
    } else {
        // First draw
        updateCharts();
    }
}

// Global exposure for the HTML onclick
window.swapCharts = swapCharts;

window.addEventListener('load', function () {
    let topNPickers = [...document.getElementsByClassName('top-picker')]
    topNPickers.forEach(e => e.addEventListener('change', () => {
        parseTopN()
        const idx = parseInt(e.attributes['data-entity'].value)
        updateCharts([idx])
    }))
    
    // Set max values for pickers
    topNPickers.sort(((a, b) => parseInt(a.attributes['data-entity'].value) - parseInt(b.attributes['data-entity'].value)))
    topNPickers.forEach(e => {
        const idx = parseInt(e.attributes['data-entity'].value)
        if (dataSources[idx]) {
            e.max = dataSources[idx].length
            e.value = Math.min(e.max, 9)
        }
    })

    parseTopN()
    togglePlaceholders(getPresentDataMask())
    updateNumTotal()
    
    // Initial Draw
    updateCharts();
    
    // Handle Resize
    window.addEventListener('resize', () => {
        Object.values(charts).forEach(c => c && c.resize());
    });
})
