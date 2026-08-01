import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import ErrorState from '../components/ErrorState';
import LoadingState from '../components/LoadingState';
import StatusBadge from '../components/StatusBadge';
import { cartAPI, getAPIErrorMessage, orderAPI, productAPI } from '../services/api';
import { formatMoney, getProductImageUrl, getStockMeta } from '../utils/display';

const ProductDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [product, setProduct] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [quantity, setQuantity] = useState(1);
  const [feedback, setFeedback] = useState(null);
  const [buying, setBuying] = useState(false);

  useEffect(() => {
    const fetchProduct = async () => {
      try {
        const data = await productAPI.getProduct(id);
        setProduct(data.product);
      } catch (fetchError) {
        console.error('Failed to fetch product:', fetchError);
        setError('获取商品详情失败');
      } finally {
        setLoading(false);
      }
    };

    fetchProduct();
  }, [id]);

  const ensureLoggedIn = () => {
    const token = localStorage.getItem('token');
    if (!token) {
      navigate('/login');
      return false;
    }
    return true;
  };

  const handleAddToCart = async () => {
    if (buying || !ensureLoggedIn() || !product) return;

    try {
      await cartAPI.addItem({
        product_id: product.id,
        quantity,
      });
      setFeedback({ type: 'success', text: '商品已加入购物车。' });
    } catch (actionError) {
      console.error('Failed to add to cart:', actionError);
      setFeedback({ type: 'error', text: '添加到购物车失败，请稍后重试。' });
    }
  };

  const handleBuyNow = async () => {
    if (buying || !ensureLoggedIn() || !product) return;

    try {
      setBuying(true);
      setFeedback(null);
      await orderAPI.createOrder({
        items: [
          {
            product_id: product.id,
            quantity,
          },
        ],
      });
      navigate('/orders');
    } catch (actionError) {
      console.error('Failed to create order:', actionError);
      setFeedback({
        type: 'error',
        text: getAPIErrorMessage(actionError, '\u521b\u5efa\u8ba2\u5355\u5931\u8d25\uff0c\u8bf7\u7a0d\u540e\u91cd\u8bd5\u3002'),
      });
    } finally {
      setBuying(false);
    }
  };

  if (loading) {
    return <LoadingState label="正在加载商品详情..." />;
  }

  if (error || !product) {
    return <ErrorState title="商品暂时不可用" description={error || '商品不存在'} />;
  }

  const stockMeta = getStockMeta(product.stock);
  const outOfStock = product.stock <= 0;

  return (
    <div className="page-stack product-detail">
      <Link to="/products" className="btn btn-ghost btn-sm">
        ← 返回商品列表
      </Link>

      <div className="product-detail-container">
        <div className="product-detail-image">
          <img src={getProductImageUrl(product.image_url, 'detail')} alt={product.name} />
        </div>

        <div className="product-detail-info">
          <div className="product-detail-summary">
            <span className="tag">{product.category || '未分类'}</span>
            <StatusBadge status={stockMeta.tone} label={stockMeta.label} />
          </div>

          <h1>{product.name}</h1>
          <p className="product-detail-price">{formatMoney(product.price_cents)}</p>

          <div className="product-detail-description">
            <h3>商品描述</h3>
            <p>{product.description || '该商品暂未补充详细描述。'}</p>
          </div>

          {feedback && <div className={`notice notice-${feedback.type}`}>{feedback.text}</div>}

          <div className="product-detail-actions">
            <div className="quantity-control" aria-label="选择数量">
              <button
                type="button"
                onClick={() => setQuantity((value) => Math.max(1, value - 1))}
                className="quantity-btn"
                disabled={quantity <= 1}
              >
                −
              </button>
              <span className="quantity">{quantity}</span>
              <button
                type="button"
                onClick={() => setQuantity((value) => Math.min(product.stock || 1, value + 1))}
                className="quantity-btn"
                disabled={outOfStock || quantity >= product.stock}
              >
                +
              </button>
            </div>
            <button type="button" className="btn btn-secondary" onClick={handleAddToCart} disabled={outOfStock}>
              加入购物车
            </button>
            <button type="button" className="btn btn-primary" onClick={handleBuyNow} disabled={outOfStock || buying}>
              {buying ? '\u521b\u5efa\u4e2d...' : '\u7acb\u5373\u8d2d\u4e70'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

export default ProductDetail;
