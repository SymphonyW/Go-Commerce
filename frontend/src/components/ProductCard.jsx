import { Link } from 'react-router-dom';
import { formatCurrency, getProductImageUrl, getStockMeta } from '../utils/display';

const ProductCard = ({ product }) => {
  const stock = getStockMeta(product.stock);

  return (
    <article className="product-card">
      <Link to={`/products/${product.id}`} className="product-media">
        <img src={getProductImageUrl(product.image_url)} alt={product.name} loading="lazy" />
      </Link>
      <div className="product-card-body">
        <div className="product-meta-row">
          <span className="tag">{product.category || '未分类'}</span>
          <span className={`stock-chip tone-${stock.tone}`}>{stock.label}</span>
        </div>
        <h3>{product.name}</h3>
        <p className="product-price">{formatCurrency(product.price)}</p>
        <Link to={`/products/${product.id}`} className="btn btn-secondary btn-sm">
          查看详情
        </Link>
      </div>
    </article>
  );
};

export default ProductCard;
