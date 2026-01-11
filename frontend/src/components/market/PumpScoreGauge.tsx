import { memo } from 'react';
import { cn } from '@/utils/cn';

interface PumpScoreGaugeProps {
  score: number;
  maxScore?: number;
  className?: string;
  showValue?: boolean;
}

const MAX_SCORE = 20000; // Support scores up to 16000+
const MIN_SCORE = -30;

export const PumpScoreGauge = memo(({ 
  score, 
  maxScore = MAX_SCORE, 
  className,
  showValue = false 
}: PumpScoreGaugeProps) => {
  const NUM_SEGMENTS = 21; // 21 bars total

  // Calculate gradient color for negative scores
  // Bars 1-10 (index 1 to 10) change based on score
  // Bar 0: Always green
  // Bar 20: Always red
  // Score 0: Bars 1-10 show balanced gradient
  // Score -30: Bars 1-10 all red
  const getGradientColor = (segmentIndex: number): string => {
    // Clamp score to valid range using maxScore
    const clampedScore = Math.max(MIN_SCORE, Math.min(maxScore, score));
    // Get base color (bc) for a bar at score 0
    const getBaseColor = (barIndex: number) => {
      const positionInFull = barIndex / 20; // 0 to 1
      const green = 220 * (1 - positionInFull);
      const red = 220 * positionInFull;
      const blue = 38 * Math.min(positionInFull, 1 - positionInFull);
      return { r: red, g: green, b: blue };
    };
    
    // For negative scores: complex gradient system
    if (clampedScore < 0) {
      // Bar 20 (last): Always red for negative scores
      if (segmentIndex === NUM_SEGMENTS - 1) {
        return 'rgb(220, 38, 38)'; // Solid red (#dc2626)
      }
      // If score < -30, all bars 0-20 are red
      if (clampedScore < MIN_SCORE) {
        if (segmentIndex >= 0 && segmentIndex <= 20) {
          return 'rgb(220, 38, 38)'; // Full red
        }
      }
      
      // Determine which range the score falls into
      let bar0BaseIndex: number;
      let gradientStartIndex: number;
      
      if (clampedScore >= -10 && clampedScore < 0) {
        // -1 to -10: Bar 0 uses bc-4, gradient bc-5 to bc-11
        bar0BaseIndex = 4;
        gradientStartIndex = 5;
      } else if (clampedScore >= -20 && clampedScore < -10) {
        // -11 to -20: Bar 0 uses bc-7, gradient bc-8 to bc-11
        bar0BaseIndex = 7;
        gradientStartIndex = 8;
      } else if (clampedScore >= MIN_SCORE && clampedScore < -20) {
        // -20 to -30: Bar 0 uses bc-10, gradient bc-10 to bc-11
        bar0BaseIndex = 10;
        gradientStartIndex = 10;
      } else {
        // Fallback (shouldn't happen, but just in case)
        bar0BaseIndex = 4;
        gradientStartIndex = 5;
      }
      
      // Bar 0 uses bc-X
      if (segmentIndex === 0) {
        const bc0 = getBaseColor(bar0BaseIndex);
        return `rgb(${Math.round(bc0.r)}, ${Math.round(bc0.g)}, ${Math.round(bc0.b)})`;
      }
      
      // Bars 1-10: Gradient from bc-start to bc-11
      if (segmentIndex >= 1 && segmentIndex <= 10) {
        const bcStart = getBaseColor(gradientStartIndex);
        const bcEnd = getBaseColor(11); // bc-11
        
        // Position within bars 1-10 (0 = bar 1, 1 = bar 10)
        const positionInRange = (segmentIndex - 1) / 9; // 0 to 1
        
        const r = Math.round(bcStart.r + (bcEnd.r - bcStart.r) * positionInRange);
        const g = Math.round(bcStart.g + (bcEnd.g - bcStart.g) * positionInRange);
        const b = Math.round(bcStart.b + (bcEnd.b - bcStart.b) * positionInRange);
        
        return `rgb(${Math.min(255, Math.max(0, r))}, ${Math.min(255, Math.max(0, g))}, ${Math.max(0, b)})`;
      } else {
        // Bars 11-20: Gradient starting from bar 11's color at score 0
        const bc11 = getBaseColor(11);
        const positionInRange = (segmentIndex - 10) / 10; // 0 to 1
        
        // Gradient from bar 11 color to red (bar 20)
        const r = Math.round(bc11.r + (220 - bc11.r) * positionInRange);
        const g = Math.round(bc11.g * (1 - positionInRange));
        const b = Math.round(bc11.b * (1 - positionInRange));
        
        return `rgb(${Math.min(255, Math.max(0, r))}, ${Math.min(255, Math.max(0, g))}, ${Math.max(0, b)})`;
      }
    }
    
    // For positive scores: Multiple ranges with different bc values for bar 20
    if (clampedScore >= 0) {
      // > 16000: All green for all bars
      if (clampedScore > 16000) {
        return 'rgb(34, 197, 94)'; // Solid green (#22c55e)
      }
      
      // Determine which bc value to use for bar 20 based on score range
      let bar20BaseIndex: number;
      
      if (clampedScore >= 8000) {
        // 8000 - 16000: Bar 20 uses bc-8
        bar20BaseIndex = 8;
      } else if (clampedScore >= 4000) {
        // 4000 - 8000: Bar 20 uses bc-9 (as specified, ignoring duplicate 2000-4000 bc-10)
        bar20BaseIndex = 9;
      } else if (clampedScore >= 2000) {
        // 2000 - 4000: Bar 20 uses bc-11
        bar20BaseIndex = 11;
      } else if (clampedScore >= 1000) {
        // 1000 - 2000: Bar 20 uses bc-12
        bar20BaseIndex = 12;
      } else if (clampedScore >= 500) {
        // 500 - 1000: Bar 20 uses bc-13
        bar20BaseIndex = 13;
      } else if (clampedScore >= 300) {
        // 300 - 500: Bar 20 uses bc-14
        bar20BaseIndex = 14;
      } else if (clampedScore >= 50) {
        // 50 - 300: Bar 20 uses bc-15
        bar20BaseIndex = 15;
      } else {
        // 0 - 50: Bar 20 uses bc-16
        bar20BaseIndex = 16;
      }
      
      // Bar 0: Always green
      if (segmentIndex === 0) {
        return 'rgb(34, 197, 94)'; // Solid green (#22c55e)
      }
      
      // Bar 20: Uses bc-X based on score range
      if (segmentIndex === NUM_SEGMENTS - 1) {
        const bc20 = getBaseColor(bar20BaseIndex);
        return `rgb(${Math.round(bc20.r)}, ${Math.round(bc20.g)}, ${Math.round(bc20.b)})`;
      }
      
      // Bars 1-19: Gradient from green (bar 0) to bc-X (bar 20)
      const greenColor = { r: 34, g: 197, b: 94 }; // Bar 0 color
      const bc20 = getBaseColor(bar20BaseIndex); // Bar 20 color
      
      // Position within bars 1-19 (0 = bar 1, 1 = bar 19)
      const positionInRange = (segmentIndex - 1) / (NUM_SEGMENTS - 2); // 0 to 1
      
      const r = Math.round(greenColor.r + (bc20.r - greenColor.r) * positionInRange);
      const g = Math.round(greenColor.g + (bc20.g - greenColor.g) * positionInRange);
      const b = Math.round(greenColor.b + (bc20.b - greenColor.b) * positionInRange);
      
      return `rgb(${Math.min(255, Math.max(0, r))}, ${Math.min(255, Math.max(0, g))}, ${Math.max(0, b)})`;
    }
    
    // Fallback (should never reach here, but TypeScript requires it)
    return 'rgb(128, 128, 128)';
  };

  return (
    <div className={cn('flex items-center gap-2', className)}>
      {/* Battery container */}
      <div className="relative">
        {/* Main battery body */}
        <div className="relative w-28 h-3 border-2 border-gray-800 bg-gray-900 overflow-hidden">
          {/* Segments - always all filled */}
          <div className="absolute inset-0 flex gap-[1px] p-0.5">
            {Array.from({ length: NUM_SEGMENTS }).map((_, i) => {
              const segmentColor = getGradientColor(i);
              
              return (
                <div
                  key={i}
                  className="flex-1 transition-colors duration-300"
                  style={{
                    backgroundColor: segmentColor,
                    opacity: 1,
                  }}
                />
              );
            })}
          </div>
        </div>
      </div>
      {showValue && (
        <span className="text-xs font-medium text-gray-400 min-w-[3.5rem] text-right">
          {score.toFixed(1)}
        </span>
      )}
    </div>
  );
});

PumpScoreGauge.displayName = 'PumpScoreGauge';
