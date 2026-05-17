const LoadingState = ({ label = '加载中...' }) => (
  <div className="loading-state" role="status" aria-live="polite">
    <span aria-hidden="true" />
    <p>{label}</p>
  </div>
);

export default LoadingState;
