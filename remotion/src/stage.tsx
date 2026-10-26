import React from 'react';
import {AbsoluteFill, Easing, Img, OffthreadVideo, Sequence, interpolate, spring, staticFile, useCurrentFrame, useVideoConfig} from 'remotion';
import type {Asset, EditProps, Shot} from './schema';
import {ensureFont} from './fonts';

/**
 * Resolve the framing in effect at time `t` (seconds, clip-relative).
 *
 * Explicit shots from an AI shot plan win. Otherwise the layout decides:
 * - two-up: show both speakers for the whole clip.
 * - focus: a single speaker, cropped at the chosen horizontal pan.
 * - dynamic: no plan given, so show both — it never guesses who is talking.
 */
export function shotAt(t: number, p: EditProps): Shot {
  const found = p.shots.find((s) => t >= s.start && t < s.end);
  if (found) return found;
  if (p.layout === 'focus') return {start: 0, end: p.duration, mode: 'full', pan: p.pan};
  return {start: 0, end: p.duration, mode: 'both'};
}

/** Horizontal source crop expressed as a CSS object-position percentage. */
function panFor(shot: Shot, fallback: number): number {
  switch (shot.mode) {
    case 'left':
      return 25;
    case 'right':
      return 75;
    default:
      return (shot.pan ?? fallback) * 100;
  }
}

type PanelProps = {media: string; pan: number; zoom: boolean; half?: 'left' | 'right'};

/**
 * One cropped view of the source video. `half` crops to the left or right half
 * of the source for stacked two-up layouts; otherwise `pan` chooses the crop.
 */
const Panel: React.FC<PanelProps> = ({media, pan, zoom, half}) => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const z = zoom ? interpolate(Math.sin((frame / fps) * 0.55), [-1, 1], [1, 1.055]) : 1;
  const position = half === 'left' ? '0% center' : half === 'right' ? '100% center' : `${pan}% center`;
  return (
    <AbsoluteFill style={{overflow: 'hidden', background: '#060912'}}>
      <OffthreadVideo
        src={staticFile(media)}
        muted
        style={{
          position: 'absolute',
          inset: 0,
          width: '100%',
          height: '100%',
          objectFit: 'cover',
          objectPosition: position,
          transform: `scale(${z})`,
          transformOrigin: `${pan}% center`,
        }}
      />
    </AbsoluteFill>
  );
};

/**
 * Source video framing. For a two-speaker "both" shot the panels are stacked
 * vertically in portrait exports and placed side by side in landscape exports.
 */
export const VideoStage: React.FC<{p: EditProps}> = ({p}) => {
  const frame = useCurrentFrame();
  const {fps, width, height} = useVideoConfig();
  const t = frame / fps;
  const shot = shotAt(t, p);
  const portrait = height > width * 1.05;
  const unit = Math.min(width, height) / 1080;
  const pad = Math.round(32 * unit);
  const radius = Math.round(26 * unit);
  const border = Math.max(2, Math.round(4 * unit));
  const panelStyle: React.CSSProperties = {position: 'absolute', borderRadius: radius, overflow: 'hidden', border: `${border}px solid #293448`};

  return (
    <AbsoluteFill style={{background: '#090A12'}}>
      {shot.mode === 'both' ? (
        portrait ? (
          <>
            <div style={{...panelStyle, left: pad, top: Math.round(height * 0.04), width: width - pad * 2, height: Math.floor(height * 0.44)}}>
              <Panel media={p.media} pan={25} zoom={false} half="left" />
            </div>
            <div style={{...panelStyle, left: pad, top: Math.round(height * 0.5), width: width - pad * 2, height: Math.floor(height * 0.44)}}>
              <Panel media={p.media} pan={75} zoom={false} half="right" />
            </div>
          </>
        ) : (
          <>
            <div style={{...panelStyle, left: pad, top: pad, width: Math.floor((width - pad * 3) / 2), height: height - pad * 2}}>
              <Panel media={p.media} pan={25} zoom={false} half="left" />
            </div>
            <div style={{...panelStyle, left: Math.floor((width + pad) / 2), top: pad, width: Math.floor((width - pad * 3) / 2), height: height - pad * 2}}>
              <Panel media={p.media} pan={75} zoom={false} half="right" />
            </div>
          </>
        )
      ) : (
        <Panel media={p.media} pan={panFor(shot, p.pan)} zoom={p.zoom} />
      )}
      <div style={{position: 'absolute', inset: 0, background: 'linear-gradient(0deg, rgba(0,0,0,0.52) 0%, rgba(0,0,0,0) 42%)', pointerEvents: 'none'}} />
    </AbsoluteFill>
  );
};

/** A B-roll overlay with a short cross-fade in and out. */
const BrollSequence: React.FC<{asset: Asset; index: number}> = ({asset, index}) => {
  const frame = useCurrentFrame();
  const {fps, width, height} = useVideoConfig();
  const from = Math.round(asset.start * fps);
  const frames = Math.max(1, Math.round(asset.duration * fps));
  const fadeFrames = Math.min(8, Math.floor(frames / 3));
  const local = frame - from;
  const opacity = interpolate(local, [0, fadeFrames, frames - fadeFrames, frames], [0, 1, 1, 0], {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'});
  const unit = Math.min(width, height) / 1080;
  const pip: React.CSSProperties = asset.mode === 'pip'
    ? {left: Math.round(40 * unit), top: Math.round(height * 0.08), width: Math.round(width * 0.55), height: Math.round(width * 0.55 * 0.5625), borderRadius: Math.round(24 * unit), boxShadow: '0 12px 40px rgba(0,0,0,0.5)'}
    : {inset: 0, width: '100%', height: '100%'};
  const isImage = /\.(png|jpe?g|webp|gif)$/i.test(asset.media);
  return (
    <Sequence key={index} from={from} durationInFrames={frames} layout="none">
      <div style={{position: 'absolute', overflow: 'hidden', opacity, ...pip}}>
        {isImage ? (
          <Img src={staticFile(asset.media)} style={{width: '100%', height: '100%', objectFit: 'cover'}} />
        ) : (
          <OffthreadVideo muted src={staticFile(asset.media)} style={{width: '100%', height: '100%', objectFit: 'cover'}} />
        )}
      </div>
    </Sequence>
  );
};

export const BrollLayer: React.FC<{p: EditProps}> = ({p}) => (
  <>
    {p.assets.map((asset, i) => (
      <BrollSequence key={`${asset.media}-${i}`} asset={asset} index={i} />
    ))}
  </>
);

/** A callout banner shown at the top of the frame for the first few seconds. */
export const GraphicsLayer: React.FC<{p: EditProps}> = ({p}) => {
  const frame = useCurrentFrame();
  const {fps, width, height} = useVideoConfig();
  const callout = p.callout.trim();
  if (!p.graphics || !callout) return null;
  const total = Math.max(1, Math.round(p.calloutSeconds * fps));
  if (frame > total) return null;
  const enter = spring({fps, frame, config: {damping: 18, mass: 0.8, stiffness: 130}});
  const exit = interpolate(frame, [total - Math.min(10, total / 2), total], [1, 0], {extrapolateLeft: 'clamp', extrapolateRight: 'clamp', easing: Easing.in(Easing.quad)});
  const unit = Math.min(width, height) / 1080;
  const font = ensureFont(p.captionFont);
  return (
    <div
      style={{
        position: 'absolute',
        top: Math.round(height * 0.05),
        left: Math.round(60 * unit),
        right: Math.round(60 * unit),
        transform: `translateY(${(1 - enter) * -70}px)`,
        opacity: enter * exit,
        background: 'rgba(12,15,28,0.92)',
        borderLeft: `${Math.round(8 * unit)}px solid ${p.accentColor}`,
        padding: `${Math.round(20 * unit)}px ${Math.round(24 * unit)}px`,
        borderRadius: Math.round(14 * unit),
        color: 'white',
        fontFamily: font.fontFamily,
        fontWeight: font.weight,
        fontSize: Math.round(48 * unit),
        lineHeight: 1.15,
        textAlign: 'center',
      }}
    >
      {callout}
    </div>
  );
};

/** Thin progress bar along the bottom edge of the frame. */
export const ProgressBar: React.FC<{p: EditProps}> = ({p}) => {
  const frame = useCurrentFrame();
  const {durationInFrames, width, height} = useVideoConfig();
  if (!p.progressBar) return null;
  const pct = clamp01(frame / Math.max(1, durationInFrames - 1)) * 100;
  const unit = Math.min(width, height) / 1080;
  return (
    <div style={{position: 'absolute', left: 0, right: 0, bottom: 0, height: Math.max(4, Math.round(8 * unit)), background: 'rgba(255,255,255,0.18)'}}>
      <div style={{width: `${pct}%`, height: '100%', background: p.accentColor}} />
    </div>
  );
};

const clamp01 = (n: number) => Math.max(0, Math.min(1, n));
