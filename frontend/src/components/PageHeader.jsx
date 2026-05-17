const PageHeader = ({ eyebrow, title, subtitle, meta, actions }) => (
  <header className="page-header">
    <div>
      {eyebrow && <p className="eyebrow">{eyebrow}</p>}
      <div className="page-header-title-row">
        <h1>{title}</h1>
        {meta && <span className="meta-pill">{meta}</span>}
      </div>
      {subtitle && <p className="page-header-subtitle">{subtitle}</p>}
    </div>
    {actions && <div className="page-header-actions">{actions}</div>}
  </header>
);

export default PageHeader;
