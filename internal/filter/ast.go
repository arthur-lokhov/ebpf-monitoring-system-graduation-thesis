package filter

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// bucketValue represents a histogram bucket value
type bucketValue struct {
	le    float64
	count float64
}

// MetricSelectorNode selects metrics by name and label matchers
type MetricSelectorNode struct {
	Name          string
	LabelMatchers map[string]string
}

func (n *MetricSelectorNode) Evaluate(ctx *EvaluationContext, metrics []*MetricValue) (*FilterResult, error) {
	var result []*MetricSeries

	// Get metrics from store if available
	if ctx.MetricStore != nil {
		series := ctx.MetricStore.GetMetrics(n.Name)
		for _, s := range series {
			if matchLabels(s.Labels, n.LabelMatchers) {
				result = append(result, s)
			}
		}
	} else {
		// Filter metrics by name and labels
		metricMap := make(map[uint64]*MetricSeries)
		for _, m := range metrics {
			if m.Name == n.Name && matchLabels(m.Labels, n.LabelMatchers) {
				labelHash := hashLabels(m.Labels)
				if _, ok := metricMap[labelHash]; !ok {
					metricMap[labelHash] = &MetricSeries{
						Name:   m.Name,
						Labels: m.Labels,
						Points: make([]MetricPoint, 0),
					}
				}
				metricMap[labelHash].Points = append(metricMap[labelHash].Points, MetricPoint{
					Value:     m.Value,
					Timestamp: m.Timestamp,
				})
			}
		}
		for _, s := range metricMap {
			result = append(result, s)
		}
	}

	return &FilterResult{Series: result}, nil
}

func (n *MetricSelectorNode) String() string {
	if len(n.LabelMatchers) == 0 {
		return n.Name
	}

	var labels []string
	for k, v := range n.LabelMatchers {
		labels = append(labels, fmt.Sprintf("%s=\"%s\"", k, v))
	}
	sort.Strings(labels)
	return fmt.Sprintf("%s{%s}", n.Name, strings.Join(labels, ","))
}

// RangeSelectorNode selects a range of data points
type RangeSelectorNode struct {
	Expr  ASTNode
	Range time.Duration
}

func (n *RangeSelectorNode) Evaluate(ctx *EvaluationContext, metrics []*MetricValue) (*FilterResult, error) {
	result, err := n.Expr.Evaluate(ctx, metrics)
	if err != nil {
		return nil, err
	}

	// Filter points by range
	cutoff := ctx.EndTime.Add(-n.Range)
	for _, series := range result.Series {
		filteredPoints := make([]MetricPoint, 0)
		for _, p := range series.Points {
			if p.Timestamp.After(cutoff) || p.Timestamp.Equal(cutoff) {
				filteredPoints = append(filteredPoints, p)
			}
		}
		series.Points = filteredPoints
	}

	return result, nil
}

func (n *RangeSelectorNode) String() string {
	return fmt.Sprintf("%s[%s]", n.Expr.String(), n.Range.String())
}

// RateNode calculates per-second rate of increase
type RateNode struct {
	Function string // rate, irate, increase
	Expr     ASTNode
}

func (n *RateNode) Evaluate(ctx *EvaluationContext, metrics []*MetricValue) (*FilterResult, error) {
	result, err := n.Expr.Evaluate(ctx, metrics)
	if err != nil {
		return nil, err
	}

	outputSeries := make([]*MetricSeries, 0, len(result.Series))

	for _, series := range result.Series {
		if len(series.Points) < 2 {
			continue
		}

		// Sort points by timestamp
		sort.Slice(series.Points, func(i, j int) bool {
			return series.Points[i].Timestamp.Before(series.Points[j].Timestamp)
		})

		// Calculate rate
		ratePoints := make([]MetricPoint, 0)
		for i := 1; i < len(series.Points); i++ {
			prev := series.Points[i-1]
			curr := series.Points[i]

			duration := curr.Timestamp.Sub(prev.Timestamp).Seconds()
			if duration <= 0 {
				continue
			}

			var rate float64
			switch n.Function {
			case "rate":
				// Calculate rate of increase, handling counter resets
				diff := curr.Value - prev.Value
				if diff < 0 {
					diff = curr.Value // Counter reset, use current value
				}
				rate = diff / duration
			case "irate":
				// Instant rate (same as rate for last two points)
				diff := curr.Value - prev.Value
				if diff < 0 {
					diff = curr.Value
				}
				rate = diff / duration
			case "increase":
				// Total increase over the range
				diff := curr.Value - prev.Value
				if diff < 0 {
					diff = curr.Value
				}
				rate = diff
			}

			ratePoints = append(ratePoints, MetricPoint{
				Value:     rate,
				Timestamp: curr.Timestamp,
			})
		}

		if len(ratePoints) > 0 {
			outputSeries = append(outputSeries, &MetricSeries{
				Name:   fmt.Sprintf("%s(%s)", n.Function, series.Name),
				Labels: series.Labels,
				Points: ratePoints,
			})
		}
	}

	return &FilterResult{Series: outputSeries}, nil
}

func (n *RateNode) String() string {
	return fmt.Sprintf("%s(%s)", n.Function, n.Expr.String())
}

// NormalizationNode normalizes metrics to per-second/minute/hour
type NormalizationNode struct {
	Function string // per_second, per_minute, per_hour
	Expr     ASTNode
}

func (n *NormalizationNode) Evaluate(ctx *EvaluationContext, metrics []*MetricValue) (*FilterResult, error) {
	result, err := n.Expr.Evaluate(ctx, metrics)
	if err != nil {
		return nil, err
	}

	var divisor float64
	switch n.Function {
	case "per_second":
		divisor = 1.0
	case "per_minute":
		divisor = 60.0
	case "per_hour":
		divisor = 3600.0
	}

	outputSeries := make([]*MetricSeries, 0, len(result.Series))
	for _, series := range result.Series {
		normalizedPoints := make([]MetricPoint, len(series.Points))
		for i, p := range series.Points {
			normalizedPoints[i] = MetricPoint{
				Value:     p.Value / divisor,
				Timestamp: p.Timestamp,
			}
		}
		outputSeries = append(outputSeries, &MetricSeries{
			Name:   fmt.Sprintf("%s(%s)", n.Function, series.Name),
			Labels: series.Labels,
			Points: normalizedPoints,
		})
	}

	return &FilterResult{Series: outputSeries}, nil
}

func (n *NormalizationNode) String() string {
	return fmt.Sprintf("%s(%s)", n.Function, n.Expr.String())
}

// AggregationNode performs aggregation operations
type AggregationNode struct {
	Operator     string // sum, avg, min, max, count
	Expr         ASTNode
	GroupingType string // by, without
	GroupLabels  []string
}

func (n *AggregationNode) Evaluate(ctx *EvaluationContext, metrics []*MetricValue) (*FilterResult, error) {
	result, err := n.Expr.Evaluate(ctx, metrics)
	if err != nil {
		return nil, err
	}

	// Group series by labels
	groups := make(map[uint64][]*MetricSeries)
	for _, series := range result.Series {
		groupKey := n.getGroupKey(series.Labels)
		groups[groupKey] = append(groups[groupKey], series)
	}

	outputSeries := make([]*MetricSeries, 0, len(groups))
	for _, group := range groups {
		if len(group) == 0 {
			continue
		}

		// Aggregate points across all series in the group
		aggregatedPoints := make([]MetricPoint, 0)
		
		// Get all unique timestamps
		timestampSet := make(map[time.Time]bool)
		for _, series := range group {
			for _, p := range series.Points {
				timestampSet[p.Timestamp] = true
			}
		}
		
		timestamps := make([]time.Time, 0, len(timestampSet))
		for ts := range timestampSet {
			timestamps = append(timestamps, ts)
		}
		sort.Slice(timestamps, func(i, j int) bool {
			return timestamps[i].Before(timestamps[j])
		})

		// Aggregate values at each timestamp
		for _, ts := range timestamps {
			var values []float64
			for _, series := range group {
				for _, p := range series.Points {
					if p.Timestamp.Equal(ts) {
						values = append(values, p.Value)
					}
				}
			}

			if len(values) == 0 {
				continue
			}

			var aggValue float64
			switch n.Operator {
			case "sum":
				for _, v := range values {
					aggValue += v
				}
			case "avg":
				for _, v := range values {
					aggValue += v
				}
				aggValue /= float64(len(values))
			case "min":
				aggValue = values[0]
				for _, v := range values[1:] {
					if v < aggValue {
						aggValue = v
					}
				}
			case "max":
				aggValue = values[0]
				for _, v := range values[1:] {
					if v > aggValue {
						aggValue = v
					}
				}
			case "count":
				aggValue = float64(len(values))
			}

			aggregatedPoints = append(aggregatedPoints, MetricPoint{
				Value:     aggValue,
				Timestamp: ts,
			})
		}

		// Use labels from first series as representative
		outputLabels := group[0].Labels
		if n.GroupingType == "by" && len(n.GroupLabels) > 0 {
			outputLabels = n.filterLabels(outputLabels, n.GroupLabels, true)
		} else if n.GroupingType == "without" && len(n.GroupLabels) > 0 {
			outputLabels = n.filterLabels(outputLabels, n.GroupLabels, false)
		}

		outputSeries = append(outputSeries, &MetricSeries{
			Name:   fmt.Sprintf("%s(%s)", n.Operator, group[0].Name),
			Labels: outputLabels,
			Points: aggregatedPoints,
		})
	}

	return &FilterResult{Series: outputSeries}, nil
}

func (n *AggregationNode) getGroupKey(labels map[string]string) uint64 {
	if n.GroupingType == "" {
		return 0 // All in one group
	}

	filtered := n.filterLabels(labels, n.GroupLabels, n.GroupingType == "by")
	return hashLabels(filtered)
}

func (n *AggregationNode) filterLabels(labels map[string]string, groupLabels []string, keep bool) map[string]string {
	result := make(map[string]string)
	
	if keep {
		// Keep only specified labels
		for _, l := range groupLabels {
			if v, ok := labels[l]; ok {
				result[l] = v
			}
		}
	} else {
		// Keep all except specified labels
		excludeSet := make(map[string]bool)
		for _, l := range groupLabels {
			excludeSet[l] = true
		}
		for k, v := range labels {
			if !excludeSet[k] {
				result[k] = v
			}
		}
	}
	
	return result
}

func (n *AggregationNode) String() string {
	if len(n.GroupLabels) == 0 {
		return fmt.Sprintf("%s(%s)", n.Operator, n.Expr.String())
	}
	return fmt.Sprintf("%s %s (%s) (%s)", n.Operator, n.GroupingType, strings.Join(n.GroupLabels, ","), n.Expr.String())
}

// HistogramQuantileNode calculates quantiles from histogram buckets
type HistogramQuantileNode struct {
	Quantile   float64
	BucketExpr ASTNode
}

func (n *HistogramQuantileNode) Evaluate(ctx *EvaluationContext, metrics []*MetricValue) (*FilterResult, error) {
	result, err := n.BucketExpr.Evaluate(ctx, metrics)
	if err != nil {
		return nil, err
	}

	// Group buckets by series (excluding le label)
	bucketGroups := make(map[uint64]map[string]*MetricSeries) // groupKey -> le value -> series
	
	for _, series := range result.Series {
		leValue, ok := series.Labels["le"]
		if !ok {
			continue
		}
		
		labelsCopy := make(map[string]string)
		for k, v := range series.Labels {
			if k != "le" {
				labelsCopy[k] = v
			}
		}
		
		groupKey := hashLabels(labelsCopy)
		if _, ok := bucketGroups[groupKey]; !ok {
			bucketGroups[groupKey] = make(map[string]*MetricSeries)
		}
		bucketGroups[groupKey][leValue] = series
	}

	outputSeries := make([]*MetricSeries, 0)
	
	for _, buckets := range bucketGroups {
		// Get all timestamps
		timestampSet := make(map[time.Time]bool)
		for _, series := range buckets {
			for _, p := range series.Points {
				timestampSet[p.Timestamp] = true
			}
		}
		
		timestamps := make([]time.Time, 0, len(timestampSet))
		for ts := range timestampSet {
			timestamps = append(timestamps, ts)
		}
		sort.Slice(timestamps, func(i, j int) bool {
			return timestamps[i].Before(timestamps[j])
		})
		
		// Calculate quantile for each timestamp
		quantilePoints := make([]MetricPoint, 0)
		for _, ts := range timestamps {
			// Collect bucket values at this timestamp
			var bucketValues []bucketValue
			
			for leStr, series := range buckets {
				for _, p := range series.Points {
					if p.Timestamp.Equal(ts) {
						le, err := strconvParseFloat(leStr)
						if err != nil {
							continue
						}
						bucketValues = append(bucketValues, bucketValue{le: le, count: p.Value})
					}
				}
			}
			
			if len(bucketValues) == 0 {
				continue
			}
			
			// Sort buckets by le value
			sort.Slice(bucketValues, func(i, j int) bool {
				return bucketValues[i].le < bucketValues[j].le
			})
			
			// Calculate quantile using linear interpolation
			quantile := n.calculateQuantile(bucketValues)
			quantilePoints = append(quantilePoints, MetricPoint{
				Value:     quantile,
				Timestamp: ts,
			})
		}
		
		if len(quantilePoints) > 0 {
			// Get labels from first bucket (without le)
			var sampleLabels map[string]string
			for _, series := range buckets {
				sampleLabels = series.Labels
				break
			}
			delete(sampleLabels, "le")
			
			outputSeries = append(outputSeries, &MetricSeries{
				Name:   fmt.Sprintf("histogram_quantile(%.2f)", n.Quantile),
				Labels: sampleLabels,
				Points: quantilePoints,
			})
		}
	}

	return &FilterResult{Series: outputSeries}, nil
}

func (n *HistogramQuantileNode) calculateQuantile(buckets []bucketValue) float64 {
	if len(buckets) == 0 {
		return 0
	}

	targetCount := n.Quantile * buckets[len(buckets)-1].count

	for i := 1; i < len(buckets); i++ {
		if buckets[i].count >= targetCount {
			// Linear interpolation between buckets[i-1] and buckets[i]
			prev := buckets[i-1]
			curr := buckets[i]

			if curr.count == prev.count {
				return curr.le
			}

			ratio := (targetCount - prev.count) / (curr.count - prev.count)
			return prev.le + ratio*(curr.le-prev.le)
		}
	}

	return buckets[len(buckets)-1].le
}

func (n *HistogramQuantileNode) String() string {
	return fmt.Sprintf("histogram_quantile(%.2f, %s)", n.Quantile, n.BucketExpr.String())
}

// LabelFunctionNode performs label manipulation
type LabelFunctionNode struct {
	Function string // label_join, label_replace
	Args     []string
	Expr     ASTNode
}

func (n *LabelFunctionNode) Evaluate(ctx *EvaluationContext, metrics []*MetricValue) (*FilterResult, error) {
	result, err := n.Expr.Evaluate(ctx, metrics)
	if err != nil {
		return nil, err
	}

	outputSeries := make([]*MetricSeries, 0, len(result.Series))
	
	for _, series := range result.Series {
		newLabels := make(map[string]string)
		for k, v := range series.Labels {
			newLabels[k] = v
		}
		
		switch n.Function {
		case "label_join":
			// label_join(destination, source1, source2, ..., separator)
			if len(n.Args) >= 3 {
				destLabel := n.Args[0]
				separator := n.Args[len(n.Args)-1]
				sourceLabels := n.Args[1 : len(n.Args)-1]
				
				var values []string
				for _, src := range sourceLabels {
					if v, ok := series.Labels[src]; ok {
						values = append(values, v)
					}
				}
				newLabels[destLabel] = strings.Join(values, separator)
			}
			
		case "label_replace":
			// label_replace(destination, replacement, source, regex)
			if len(n.Args) >= 4 {
				destLabel := n.Args[0]
				replacement := n.Args[1]
				sourceLabel := n.Args[2]
				regex := n.Args[3]
				
				if v, ok := series.Labels[sourceLabel]; ok {
					if matched, _ := regexpMatch(regex, v); matched {
						newLabels[destLabel] = replacement
					}
				}
			}
		}
		
		outputSeries = append(outputSeries, &MetricSeries{
			Name:   series.Name,
			Labels: newLabels,
			Points: series.Points,
		})
	}

	return &FilterResult{Series: outputSeries}, nil
}

func (n *LabelFunctionNode) String() string {
	return fmt.Sprintf("%s(%s, %s)", n.Function, strings.Join(n.Args, ","), n.Expr.String())
}

// BinaryOpNode represents binary operations
type BinaryOpNode struct {
	Op    string
	Left  ASTNode
	Right ASTNode
}

func (n *BinaryOpNode) Evaluate(ctx *EvaluationContext, metrics []*MetricValue) (*FilterResult, error) {
	leftResult, err := n.Left.Evaluate(ctx, metrics)
	if err != nil {
		return nil, err
	}

	rightResult, err := n.Right.Evaluate(ctx, metrics)
	if err != nil {
		return nil, err
	}

	outputSeries := make([]*MetricSeries, 0)
	
	// Match series by labels and apply operation
	for _, leftSeries := range leftResult.Series {
		for _, rightSeries := range rightResult.Series {
			if !labelMapsEqual(leftSeries.Labels, rightSeries.Labels) {
				continue
			}
			
			points := make([]MetricPoint, 0)
			for _, lp := range leftSeries.Points {
				for _, rp := range rightSeries.Points {
					if lp.Timestamp.Equal(rp.Timestamp) {
						var value float64
						switch n.Op {
						case "+":
							value = lp.Value + rp.Value
						case "-":
							value = lp.Value - rp.Value
						case "*":
							value = lp.Value * rp.Value
						case "/":
							if rp.Value != 0 {
								value = lp.Value / rp.Value
							} else {
								value = math.NaN()
							}
						}
						points = append(points, MetricPoint{
							Value:     value,
							Timestamp: lp.Timestamp,
						})
					}
				}
			}
			
			if len(points) > 0 {
				outputSeries = append(outputSeries, &MetricSeries{
					Name:   fmt.Sprintf("(%s %s %s)", leftSeries.Name, n.Op, rightSeries.Name),
					Labels: leftSeries.Labels,
					Points: points,
				})
			}
		}
	}

	return &FilterResult{Series: outputSeries}, nil
}

func (n *BinaryOpNode) String() string {
	return fmt.Sprintf("(%s %s %s)", n.Left.String(), n.Op, n.Right.String())
}

// Helper functions

func matchLabels(labels, matchers map[string]string) bool {
	if len(matchers) == 0 {
		return true
	}
	for k, v := range matchers {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func labelMapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func regexpMatch(pattern, s string) (bool, error) {
	// Simple pattern matching - in production use regexp.Compile
	return strings.Contains(s, pattern), nil
}

func strconvParseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
