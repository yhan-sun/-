export function usePlayerMiniWindow(React, viewing, videoRef) {
  const [mini, setMini] = React.useState(false);
  const requested = React.useRef(false);
  const queue = React.useRef(Promise.resolve());
  const mounted = React.useRef(true);
  const change = React.useCallback(enabled => {
    if (requested.current === enabled) return queue.current;
    requested.current = enabled;
    const video = videoRef.current;
    const aspect = video?.videoHeight > 0 ? video.videoWidth / video.videoHeight : 0;
    const run = queue.current.catch(() => {}).then(async () => {
      try {
        const active = await window.electronAPI.setPlayerMiniWindow(enabled, aspect);
        if (mounted.current) setMini(active);
      } catch (error) {
        if (requested.current === enabled) requested.current = !enabled;
        throw error;
      }
    });
    queue.current = run;
    return run;
  }, [videoRef]);

  React.useEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; if (requested.current) change(false).catch(console.warn); };
  }, [change]);
  React.useEffect(() => {
    if (!viewing && requested.current) change(false).catch(console.warn);
  }, [viewing, change]);

  async function toggle() {
    const video = videoRef.current;
    if (!video) return;
    if (requested.current || mini) return change(false);
    const doc = video.ownerDocument;
    if (doc.pictureInPictureElement === video) return doc.exitPictureInPicture();
    if (video.webkitPresentationMode === 'picture-in-picture') return video.webkitSetPresentationMode('inline');
    if (window.electronAPI.pictureInPictureMode === 'native' && await window.electronAPI.enterNativePictureInPicture(video)) return;
    if (doc.fullscreenElement && doc.exitFullscreen) await doc.exitFullscreen();
    return change(true);
  }
  return { mini, toggle };
}
