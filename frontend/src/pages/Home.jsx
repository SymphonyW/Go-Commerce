import { useEffect, useState } from 'react';
import { productAPI } from '../services/api';
import { Link } from 'react-router-dom';

const Home = () => {
  const [products, setProducts] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchProducts = async () => {
      try {
        const data = await productAPI.listProducts();
        setProducts(data.products || []);
      } catch (error) {
        console.error('Failed to fetch products:', error);
      } finally {
        setLoading(false);
      }
    };

    fetchProducts();
  }, []);

  if (loading) {
    return <div className="loading">加载中...</div>;
  }

  return (
    <div className="home">
      <div className="hero">
        <h1>欢迎来到 Go Commerce</h1>
        <p>探索我们的精选商品</p>
        <Link to="/products" className="btn">
          查看商品
        </Link>
      </div>

      <div className="featured-products">
        <h2>热门商品</h2>
        <div className="products-grid">
          {products.slice(0, 4).map((product) => (
            <div key={product.id} className="product-card">
              <img 
                src={product.imageUrl || 'https://via.placeholder.com/200'} 
                alt={product.name} 
                className="product-image"
              />
              <h3>{product.name}</h3>
              <p className="product-price">¥{product.price}</p>
              <Link to={`/products/${product.id}`} className="btn btn-sm">
                查看详情
              </Link>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};

export default Home;